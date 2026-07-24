package retrieval

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"s26.sh/tok/internal/storage"
)

const (
	DefaultLimit       = 10
	DefaultMaxFileSize = 512 * 1024
)

var tokenPattern = regexp.MustCompile(`[\pL\pN_]+`)

type Store interface {
	ReplaceIndexedDocuments(ctx context.Context, projectID int64, docs []storage.IndexedDocumentInput) (int, error)
	ListIndexedDocuments(ctx context.Context, projectID int64) ([]storage.IndexedDocument, error)
	UpsertIndexMetadata(ctx context.Context, projectID int64, sourceID sql.NullInt64, key, value string) (storage.IndexMetadata, error)
	ListProjectIndexMetadata(ctx context.Context, projectID int64) ([]storage.IndexMetadata, error)
}

type Service struct {
	store       Store
	maxFileSize int64
}

type IndexSummary struct {
	ProjectName      string
	IndexedDocuments int
	SkippedFiles     int
	SkippedReasons   map[string]int
	UpdatedAt        string
}

type IndexStatus struct {
	ProjectName      string
	IndexedDocuments int
	SkippedFiles     int
	SkippedReasons   map[string]int
	UpdatedAt        string
}

type SearchResult struct {
	Path       string
	Score      float64
	Line       int
	Snippet    string
	Excerpt    string
	Provenance string
}

func NewService(store Store) *Service {
	return &Service{
		store:       store,
		maxFileSize: DefaultMaxFileSize,
	}
}

func (s *Service) IndexProject(ctx context.Context, project storage.Project) (IndexSummary, error) {
	if s == nil || s.store == nil {
		return IndexSummary{}, errors.New("retrieval service store is required")
	}
	if project.ID <= 0 {
		return IndexSummary{}, errors.New("project id is required")
	}
	if strings.TrimSpace(project.Path) == "" {
		return IndexSummary{}, errors.New("project path is required")
	}

	discovery, err := discoverFiles(project.Path, s.maxFileSize)
	if err != nil {
		return IndexSummary{}, err
	}

	docs := make([]storage.IndexedDocumentInput, 0, len(discovery.files))
	for _, file := range discovery.files {
		content, err := os.ReadFile(file.absPath)
		if err != nil {
			return IndexSummary{}, fmt.Errorf("read project file %q: %w", file.relPath, err)
		}
		docs = append(docs, storage.IndexedDocumentInput{
			ProjectID:  project.ID,
			Path:       file.relPath,
			Provenance: "project_file",
			SizeBytes:  int64(len(content)),
			Content:    string(content),
		})
	}

	indexed, err := s.store.ReplaceIndexedDocuments(ctx, project.ID, docs)
	if err != nil {
		return IndexSummary{}, err
	}

	summary := IndexSummary{
		ProjectName:      project.Name,
		IndexedDocuments: indexed,
		SkippedFiles:     discovery.skippedFiles(),
		SkippedReasons:   cloneReasonCounts(discovery.skippedReasons),
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := s.storeIndexSummary(ctx, project.ID, summary); err != nil {
		return IndexSummary{}, err
	}

	return summary, nil
}

func (s *Service) IndexStatus(ctx context.Context, project storage.Project) (IndexStatus, error) {
	if s == nil || s.store == nil {
		return IndexStatus{}, errors.New("retrieval service store is required")
	}
	if project.ID <= 0 {
		return IndexStatus{}, errors.New("project id is required")
	}

	metadata, err := s.store.ListProjectIndexMetadata(ctx, project.ID)
	if err != nil {
		return IndexStatus{}, err
	}

	status := IndexStatus{
		ProjectName:    project.Name,
		SkippedReasons: map[string]int{},
	}
	for _, item := range metadata {
		switch item.Key {
		case "retrieval_documents":
			status.IndexedDocuments, _ = strconv.Atoi(item.Value)
			if status.UpdatedAt == "" {
				status.UpdatedAt = item.UpdatedAt
			}
		case "skipped_files":
			status.SkippedFiles, _ = strconv.Atoi(item.Value)
		case "skipped_reasons":
			_ = json.Unmarshal([]byte(item.Value), &status.SkippedReasons)
		case "indexed_at":
			status.UpdatedAt = item.Value
		}
	}
	if status.SkippedReasons == nil {
		status.SkippedReasons = map[string]int{}
	}

	return status, nil
}

func (s *Service) storeIndexSummary(ctx context.Context, projectID int64, summary IndexSummary) error {
	reasonsJSON, err := json.Marshal(summary.SkippedReasons)
	if err != nil {
		return fmt.Errorf("encode skipped reasons: %w", err)
	}

	metadata := map[string]string{
		"retrieval_documents": strconv.Itoa(summary.IndexedDocuments),
		"skipped_files":       strconv.Itoa(summary.SkippedFiles),
		"skipped_reasons":     string(reasonsJSON),
		"indexed_at":          summary.UpdatedAt,
	}
	for key, value := range metadata {
		if _, err := s.store.UpsertIndexMetadata(ctx, projectID, sql.NullInt64{}, key, value); err != nil {
			return err
		}
	}
	return nil
}

func cloneReasonCounts(input map[string]int) map[string]int {
	output := make(map[string]int, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func (s *Service) Search(ctx context.Context, project storage.Project, query string, limit int) ([]SearchResult, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("retrieval service store is required")
	}
	if project.ID <= 0 {
		return nil, errors.New("project id is required")
	}
	tokens := Tokenize(query)
	if len(tokens) == 0 {
		return nil, errors.New("search query must contain at least one token")
	}
	if limit <= 0 {
		limit = DefaultLimit
	}

	docs, err := s.store.ListIndexedDocuments(ctx, project.ID)
	if err != nil {
		return nil, err
	}

	ranked := rankDocuments(docs, tokens)
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}

	results := make([]SearchResult, 0, len(ranked))
	for _, rankedDoc := range ranked {
		line, snippet, excerpt := matchingLine(rankedDoc.Content, tokens)
		results = append(results, SearchResult{
			Path:       rankedDoc.Path,
			Score:      rankedDoc.Score,
			Line:       line,
			Snippet:    snippet,
			Excerpt:    excerpt,
			Provenance: rankedDoc.Provenance,
		})
	}

	return results, nil
}

func Tokenize(input string) []string {
	matches := tokenPattern.FindAllString(input, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(matches))
	tokens := make([]string, 0, len(matches))
	for _, match := range matches {
		for _, token := range expandToken(match) {
			if _, ok := seen[token]; ok {
				continue
			}
			seen[token] = struct{}{}
			tokens = append(tokens, token)
		}
	}
	return tokens
}

type discoveredFile struct {
	absPath string
	relPath string
}

type discoverySummary struct {
	files          []discoveredFile
	skippedReasons map[string]int
}

func (s discoverySummary) skippedFiles() int {
	var total int
	for _, count := range s.skippedReasons {
		total += count
	}
	return total
}

func discoverFiles(root string, maxFileSize int64) (discoverySummary, error) {
	summary := discoverySummary{
		skippedReasons: map[string]int{},
	}
	ignoreRules, err := loadIgnoreRules(root)
	if err != nil {
		return discoverySummary{}, err
	}

	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}

		name := entry.Name()
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)

		if entry.IsDir() {
			if shouldSkipDir(name) {
				summary.addSkip("ignored_directory")
				return filepath.SkipDir
			}
			if ignoreRules.Ignore(relPath, true) {
				summary.addSkip("gitignore")
				return filepath.SkipDir
			}
			return nil
		}

		if ignoreRules.Ignore(relPath, false) {
			summary.addSkip("gitignore")
			return nil
		}

		if !shouldIndexFile(relPath, name) {
			summary.addSkip(skipFileReason(relPath, name))
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxFileSize {
			summary.addSkip("too_large")
			return nil
		}

		text, err := looksTextFile(path)
		if err != nil {
			return err
		}
		if !text {
			summary.addSkip("binary_or_non_utf8")
			return nil
		}

		summary.files = append(summary.files, discoveredFile{absPath: path, relPath: relPath})
		return nil
	})
	if err != nil {
		return discoverySummary{}, fmt.Errorf("discover project files: %w", err)
	}

	sort.Slice(summary.files, func(i, j int) bool {
		return summary.files[i].relPath < summary.files[j].relPath
	})

	return summary, nil
}

func (s discoverySummary) addSkip(reason string) {
	if reason == "" {
		reason = "other"
	}
	s.skippedReasons[reason]++
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn", "node_modules", "vendor", "dist", "build", "coverage", ".cache", ".tok":
		return true
	default:
		return strings.HasPrefix(name, ".")
	}
}

func shouldIndexFile(relPath, name string) bool {
	if strings.HasPrefix(name, ".env") {
		return false
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".go", ".mod", ".sum", ".md", ".txt", ".yaml", ".yml", ".json", ".toml", ".sql", ".sh":
		return true
	default:
		return relPath == "Makefile" || relPath == "Dockerfile"
	}
}

func skipFileReason(relPath, name string) string {
	if strings.HasPrefix(name, ".env") {
		return "env_file"
	}
	return "unsupported_extension"
}

type ignoreRules []ignoreRule

type ignoreRule struct {
	pattern  string
	negated  bool
	dirOnly  bool
	anchored bool
}

func loadIgnoreRules(root string) (ignoreRules, error) {
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read .gitignore: %w", err)
	}

	var rules ignoreRules
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, `\#`) {
			line = strings.TrimPrefix(line, `\`)
		}

		rule := ignoreRule{}
		if strings.HasPrefix(line, "!") {
			rule.negated = true
			line = strings.TrimPrefix(line, "!")
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = filepath.ToSlash(line)
		rule.anchored = strings.HasPrefix(line, "/")
		line = strings.TrimPrefix(line, "/")
		rule.dirOnly = strings.HasSuffix(line, "/")
		line = strings.TrimSuffix(line, "/")
		if line == "" {
			continue
		}
		rule.pattern = line
		rules = append(rules, rule)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse .gitignore: %w", err)
	}

	return rules, nil
}

func (rules ignoreRules) Ignore(relPath string, isDir bool) bool {
	relPath = filepath.ToSlash(strings.TrimPrefix(relPath, "./"))
	var ignored bool
	for _, rule := range rules {
		if rule.matches(relPath, isDir) {
			ignored = !rule.negated
		}
	}
	return ignored
}

func (rule ignoreRule) matches(relPath string, isDir bool) bool {
	if rule.dirOnly && !isDir {
		return false
	}
	pattern := rule.pattern
	if pattern == "" || relPath == "" {
		return false
	}

	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return relPath == prefix || strings.HasPrefix(relPath, prefix+"/")
	}

	if rule.anchored {
		return matchPathPattern(pattern, relPath)
	}

	if !strings.Contains(pattern, "/") {
		for _, part := range strings.Split(relPath, "/") {
			if matchPathPattern(pattern, part) {
				return true
			}
		}
		return false
	}

	if matchPathPattern(pattern, relPath) {
		return true
	}
	parts := strings.Split(relPath, "/")
	for i := 1; i < len(parts); i++ {
		if matchPathPattern(pattern, strings.Join(parts[i:], "/")) {
			return true
		}
	}
	return false
}

func matchPathPattern(pattern, value string) bool {
	if pattern == value {
		return true
	}
	matched, err := filepath.Match(pattern, value)
	return err == nil && matched
}

func looksTextFile(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	buf := make([]byte, 4096)
	n, err := reader.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	if n == 0 {
		return true, nil
	}
	buf = buf[:n]
	if strings.ContainsRune(string(buf), '\x00') {
		return false, nil
	}
	return utf8.Valid(buf), nil
}

type rankedDocument struct {
	storage.IndexedDocument
	Score float64
}

func rankDocuments(docs []storage.IndexedDocument, queryTokens []string) []rankedDocument {
	if len(docs) == 0 || len(queryTokens) == 0 {
		return nil
	}

	const (
		k1 = 1.2
		b  = 0.75
	)

	type documentStats struct {
		doc         storage.IndexedDocument
		frequencies map[string]int
		length      int
	}

	stats := make([]documentStats, 0, len(docs))
	documentFrequency := make(map[string]int, len(queryTokens))
	totalLength := 0
	for _, doc := range docs {
		contentTokens := tokenizeAll(doc.Content)
		frequencies := make(map[string]int, len(contentTokens))
		for _, token := range contentTokens {
			frequencies[token]++
		}

		for _, queryToken := range queryTokens {
			if frequencies[queryToken] > 0 {
				documentFrequency[queryToken]++
			}
		}

		length := len(contentTokens)
		totalLength += length
		stats = append(stats, documentStats{
			doc:         doc,
			frequencies: frequencies,
			length:      length,
		})
	}

	avgLength := float64(totalLength) / float64(len(stats))
	if avgLength == 0 {
		avgLength = 1
	}

	results := make([]rankedDocument, 0, len(stats))
	n := float64(len(stats))
	for _, stat := range stats {
		var score float64
		for _, queryToken := range queryTokens {
			tf := float64(stat.frequencies[queryToken])
			if tf == 0 {
				continue
			}
			df := float64(documentFrequency[queryToken])
			idf := math.Log1p((n - df + 0.5) / (df + 0.5))
			docLength := float64(stat.length)
			score += idf * ((tf * (k1 + 1)) / (tf + k1*(1-b+b*(docLength/avgLength))))
		}
		if score > 0 {
			results = append(results, rankedDocument{
				IndexedDocument: stat.doc,
				Score:           score,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Path < results[j].Path
		}
		return results[i].Score > results[j].Score
	})

	return results
}

func tokenizeAll(input string) []string {
	matches := tokenPattern.FindAllString(input, -1)
	tokens := make([]string, 0, len(matches))
	for _, match := range matches {
		tokens = append(tokens, expandToken(match)...)
	}
	return tokens
}

func expandToken(raw string) []string {
	lower := strings.ToLower(raw)
	tokens := []string{lower}
	for _, part := range splitIdentifier(raw) {
		part = strings.ToLower(part)
		if part != "" && part != lower {
			tokens = append(tokens, part)
		}
	}
	return tokens
}

func splitIdentifier(raw string) []string {
	chunks := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '_'
	})

	var parts []string
	for _, chunk := range chunks {
		if chunk == "" {
			continue
		}

		runes := []rune(chunk)
		start := 0
		for i := 1; i < len(runes); i++ {
			prev := runes[i-1]
			curr := runes[i]
			nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if unicode.IsLower(prev) && unicode.IsUpper(curr) || unicode.IsUpper(prev) && unicode.IsUpper(curr) && nextIsLower {
				parts = append(parts, string(runes[start:i]))
				start = i
			}
		}
		parts = append(parts, string(runes[start:]))
	}
	return parts
}

func matchingLine(content string, tokens []string) (int, string, string) {
	lines := strings.Split(content, "\n")
	bestLine := 0
	bestScore := 0
	for idx, line := range lines {
		lower := strings.ToLower(line)
		score := 0
		for _, token := range tokens {
			if strings.Contains(lower, token) {
				score++
			}
		}
		if score > bestScore {
			bestLine = idx + 1
			bestScore = score
		}
	}
	if bestScore > 0 {
		return bestLine, trimSnippet(lines[bestLine-1]), excerpt(lines, bestLine, 1)
	}
	if len(lines) == 0 {
		return 0, "", ""
	}
	return 1, trimSnippet(lines[0]), excerpt(lines, 1, 1)
}

func trimSnippet(line string) string {
	snippet := strings.Join(strings.Fields(line), " ")
	if len(snippet) <= 160 {
		return snippet
	}
	return snippet[:157] + "..."
}

func excerpt(lines []string, lineNumber, radius int) string {
	if len(lines) == 0 || lineNumber <= 0 {
		return ""
	}
	start := lineNumber - radius
	if start < 1 {
		start = 1
	}
	end := lineNumber + radius
	if end > len(lines) {
		end = len(lines)
	}

	var out strings.Builder
	for i := start; i <= end; i++ {
		if out.Len() > 0 {
			out.WriteString("\n")
		}
		fmt.Fprintf(&out, "%d: %s", i, trimSnippet(lines[i-1]))
	}
	return out.String()
}
