package retrieval

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	bleve "github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/bmatcuk/doublestar/v4"

	"s26.sh/tok/internal/storage"
)

const (
	DefaultLimit       = 10
	DefaultMaxFileSize = 512 * 1024

	StateNeverIndexed = "never_indexed"
	StatePathMissing  = "path_missing"
	StateReady        = "ready"
	StateStale        = "stale"
	StateFailed       = "failed"
)

const (
	chunkMaxLines = 80
	chunkOverlap  = 10
)

var tokenPattern = regexp.MustCompile(`[\pL\pN_]+`)

type Store interface {
	DataDir() string
	UpsertIndexMetadata(ctx context.Context, projectID int64, sourceID sql.NullInt64, key, value string) (storage.IndexMetadata, error)
	ListProjectIndexMetadata(ctx context.Context, projectID int64) ([]storage.IndexMetadata, error)
	ReplaceIndexFileManifest(ctx context.Context, projectID int64, files []storage.IndexFileManifestInput) error
	ListIndexFileManifest(ctx context.Context, projectID int64) ([]storage.IndexFileManifest, error)
	UpsertIndexPolicy(ctx context.Context, projectID int64, includePatterns, ignorePatterns []string) (storage.IndexPolicy, error)
	GetIndexPolicy(ctx context.Context, projectID int64) (storage.IndexPolicy, error)
}

type Service struct {
	store       Store
	maxFileSize int64
	indexRoot   string
}

type IndexSummary struct {
	ProjectName      string
	State            string
	PathExists       bool
	IndexedDocuments int
	IndexedChunks    int
	SkippedFiles     int
	SkippedReasons   map[string]int
	UpdatedAt        string
	LastError        string
}

type IndexStatus struct {
	ProjectName      string
	State            string
	PathExists       bool
	IndexedDocuments int
	IndexedChunks    int
	SkippedFiles     int
	SkippedReasons   map[string]int
	UpdatedAt        string
	LastError        string
}

type SearchResult struct {
	Path       string
	Score      float64
	Line       int
	LineStart  int
	LineEnd    int
	Snippet    string
	Excerpt    string
	Provenance string
}

type IndexPolicy struct {
	ProjectName      string
	IncludePatterns  []string
	IgnorePatterns   []string
	CreatedAt        string
	UpdatedAt        string
	SeededFromIgnore bool
}

type indexedChunk struct {
	ID         string `json:"id"`
	ProjectID  int64  `json:"project_id"`
	Path       string `json:"path"`
	Ext        string `json:"ext"`
	Content    string `json:"content"`
	SearchText string `json:"search_content"`
	LineStart  int    `json:"line_start"`
	LineEnd    int    `json:"line_end"`
	Provenance string `json:"provenance"`
}

func NewService(store Store) *Service {
	service := &Service{
		store:       store,
		maxFileSize: DefaultMaxFileSize,
	}
	if store != nil && strings.TrimSpace(store.DataDir()) != "" {
		service.indexRoot = filepath.Join(store.DataDir(), "indexes")
	}
	return service
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

	if err := ensureProjectPath(project.Path); err != nil {
		state := StateFailed
		pathExists := false
		if errors.Is(err, os.ErrNotExist) {
			state = StatePathMissing
		}
		summary := IndexSummary{
			ProjectName:    project.Name,
			State:          state,
			PathExists:     pathExists,
			SkippedReasons: map[string]int{},
			UpdatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
			LastError:      err.Error(),
		}
		if saveErr := s.storeIndexSummary(ctx, project.ID, summary); saveErr != nil {
			return IndexSummary{}, saveErr
		}
		return summary, nil
	}

	policy, err := s.ensureIndexPolicy(ctx, project)
	if err != nil {
		return IndexSummary{}, err
	}

	discovery, err := discoverFiles(project.Path, s.maxFileSize, policy.IgnorePatterns)
	if err != nil {
		summary := IndexSummary{
			ProjectName:    project.Name,
			State:          StateFailed,
			PathExists:     true,
			SkippedReasons: map[string]int{},
			UpdatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
			LastError:      err.Error(),
		}
		if saveErr := s.storeIndexSummary(ctx, project.ID, summary); saveErr != nil {
			return IndexSummary{}, saveErr
		}
		return summary, nil
	}

	chunks := make([]indexedChunk, 0, len(discovery.files))
	manifest := make([]storage.IndexFileManifestInput, 0, len(discovery.files))
	for _, file := range discovery.files {
		content, err := os.ReadFile(file.absPath)
		if err != nil {
			return IndexSummary{}, fmt.Errorf("read project file %q: %w", file.relPath, err)
		}
		fileChunks := chunkFile(project.ID, file.relPath, string(content))
		chunks = append(chunks, fileChunks...)
		manifest = append(manifest, storage.IndexFileManifestInput{
			ProjectID:     project.ID,
			Path:          file.relPath,
			SizeBytes:     file.sizeBytes,
			ModTime:       file.modTime,
			ContentHash:   sha256Hex(content),
			IndexedChunks: len(fileChunks),
		})
	}

	if err := s.rebuildBleveIndex(project.ID, chunks); err != nil {
		return IndexSummary{}, err
	}
	if err := s.store.ReplaceIndexFileManifest(ctx, project.ID, manifest); err != nil {
		return IndexSummary{}, err
	}

	summary := IndexSummary{
		ProjectName:      project.Name,
		State:            StateReady,
		PathExists:       true,
		IndexedDocuments: len(discovery.files),
		IndexedChunks:    len(chunks),
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
		State:          StateNeverIndexed,
		SkippedReasons: map[string]int{},
	}
	for _, item := range metadata {
		switch item.Key {
		case "retrieval_documents":
			status.IndexedDocuments, _ = strconv.Atoi(item.Value)
			if status.UpdatedAt == "" {
				status.UpdatedAt = item.UpdatedAt
			}
		case "retrieval_chunks":
			status.IndexedChunks, _ = strconv.Atoi(item.Value)
		case "index_state":
			status.State = item.Value
		case "path_exists":
			status.PathExists = item.Value == "true"
		case "skipped_files":
			status.SkippedFiles, _ = strconv.Atoi(item.Value)
		case "skipped_reasons":
			_ = json.Unmarshal([]byte(item.Value), &status.SkippedReasons)
		case "indexed_at":
			status.UpdatedAt = item.Value
		case "last_error":
			status.LastError = item.Value
		}
	}
	if status.SkippedReasons == nil {
		status.SkippedReasons = map[string]int{}
	}

	if err := ensureProjectPath(project.Path); err != nil {
		status.PathExists = false
		status.State = StatePathMissing
		if status.LastError == "" {
			status.LastError = err.Error()
		}
		return status, nil
	}
	status.PathExists = true
	if status.UpdatedAt == "" {
		status.State = StateNeverIndexed
		return status, nil
	}
	if status.State == StateReady {
		stale, err := s.projectIndexIsStale(ctx, project)
		if err != nil {
			status.State = StateFailed
			status.LastError = err.Error()
			return status, nil
		}
		if stale {
			status.State = StateStale
		}
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
		"retrieval_chunks":    strconv.Itoa(summary.IndexedChunks),
		"index_state":         summary.State,
		"path_exists":         strconv.FormatBool(summary.PathExists),
		"skipped_files":       strconv.Itoa(summary.SkippedFiles),
		"skipped_reasons":     string(reasonsJSON),
		"indexed_at":          summary.UpdatedAt,
		"last_error":          summary.LastError,
	}
	for key, value := range metadata {
		if _, err := s.store.UpsertIndexMetadata(ctx, projectID, sql.NullInt64{}, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) projectIndexIsStale(ctx context.Context, project storage.Project) (bool, error) {
	policy, err := s.ensureIndexPolicy(ctx, project)
	if err != nil {
		return false, err
	}
	current, err := discoverFiles(project.Path, s.maxFileSize, policy.IgnorePatterns)
	if err != nil {
		return false, err
	}
	manifest, err := s.store.ListIndexFileManifest(ctx, project.ID)
	if err != nil {
		return false, err
	}
	if len(current.files) != len(manifest) {
		return true, nil
	}
	seen := make(map[string]discoveredFile, len(current.files))
	for _, file := range current.files {
		seen[file.relPath] = file
	}
	for _, file := range manifest {
		currentFile, ok := seen[file.Path]
		if !ok || currentFile.sizeBytes != file.SizeBytes || currentFile.modTime != file.ModTime {
			return true, nil
		}
	}
	return false, nil
}

func cloneReasonCounts(input map[string]int) map[string]int {
	output := make(map[string]int, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func (s *Service) IgnorePolicy(ctx context.Context, project storage.Project) (IndexPolicy, error) {
	if s == nil || s.store == nil {
		return IndexPolicy{}, errors.New("retrieval service store is required")
	}
	return s.ensureIndexPolicy(ctx, project)
}

func (s *Service) RefreshIgnorePolicy(ctx context.Context, project storage.Project) (IndexPolicy, error) {
	if s == nil || s.store == nil {
		return IndexPolicy{}, errors.New("retrieval service store is required")
	}
	if project.ID <= 0 {
		return IndexPolicy{}, errors.New("project id is required")
	}
	patterns, err := loadGitignorePatterns(project.Path)
	if err != nil {
		return IndexPolicy{}, err
	}
	existing, err := s.store.GetIndexPolicy(ctx, project.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return IndexPolicy{}, err
	}
	stored, err := s.store.UpsertIndexPolicy(ctx, project.ID, existing.IncludePatterns, patterns)
	if err != nil {
		return IndexPolicy{}, err
	}
	return indexPolicyFromStorage(project.Name, stored, true), nil
}

func (s *Service) AddIgnorePattern(ctx context.Context, project storage.Project, pattern string) (IndexPolicy, error) {
	if strings.TrimSpace(pattern) == "" {
		return IndexPolicy{}, errors.New("ignore pattern is required")
	}
	policy, err := s.ensureIndexPolicy(ctx, project)
	if err != nil {
		return IndexPolicy{}, err
	}
	ignorePatterns := append([]string{}, policy.IgnorePatterns...)
	ignorePatterns = append(ignorePatterns, pattern)
	stored, err := s.store.UpsertIndexPolicy(ctx, project.ID, policy.IncludePatterns, normalizePatterns(ignorePatterns))
	if err != nil {
		return IndexPolicy{}, err
	}
	return indexPolicyFromStorage(project.Name, stored, false), nil
}

func (s *Service) RemoveIgnorePattern(ctx context.Context, project storage.Project, pattern string) (IndexPolicy, error) {
	pattern = strings.TrimSpace(filepath.ToSlash(pattern))
	if pattern == "" {
		return IndexPolicy{}, errors.New("ignore pattern is required")
	}
	policy, err := s.ensureIndexPolicy(ctx, project)
	if err != nil {
		return IndexPolicy{}, err
	}
	ignorePatterns := make([]string, 0, len(policy.IgnorePatterns))
	for _, existing := range policy.IgnorePatterns {
		if existing != pattern {
			ignorePatterns = append(ignorePatterns, existing)
		}
	}
	stored, err := s.store.UpsertIndexPolicy(ctx, project.ID, policy.IncludePatterns, ignorePatterns)
	if err != nil {
		return IndexPolicy{}, err
	}
	return indexPolicyFromStorage(project.Name, stored, false), nil
}

func (s *Service) ensureIndexPolicy(ctx context.Context, project storage.Project) (IndexPolicy, error) {
	if project.ID <= 0 {
		return IndexPolicy{}, errors.New("project id is required")
	}
	stored, err := s.store.GetIndexPolicy(ctx, project.ID)
	if err == nil {
		return indexPolicyFromStorage(project.Name, stored, false), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return IndexPolicy{}, err
	}
	return s.RefreshIgnorePolicy(ctx, project)
}

func indexPolicyFromStorage(projectName string, policy storage.IndexPolicy, seeded bool) IndexPolicy {
	return IndexPolicy{
		ProjectName:      projectName,
		IncludePatterns:  append([]string{}, policy.IncludePatterns...),
		IgnorePatterns:   append([]string{}, policy.IgnorePatterns...),
		CreatedAt:        policy.CreatedAt,
		UpdatedAt:        policy.UpdatedAt,
		SeededFromIgnore: seeded,
	}
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

	indexPath := s.projectIndexPath(project.ID)
	if _, err := os.Stat(indexPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("inspect project index: %w", err)
	}
	index, err := bleve.Open(indexPath)
	if err != nil {
		return nil, fmt.Errorf("open project index: %w", err)
	}
	defer index.Close()

	queryParts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		queryParts = append(queryParts, "search_content:"+token)
	}
	searchQuery := bleve.NewQueryStringQuery(strings.Join(queryParts, " "))
	request := bleve.NewSearchRequestOptions(searchQuery, limit, 0, false)
	request.Fields = []string{"path", "content", "line_start", "line_end", "provenance"}

	searchResult, err := index.SearchInContext(ctx, request)
	if err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(searchResult.Hits))
	for _, hit := range searchResult.Hits {
		content := stringField(hit.Fields, "content")
		lineStart := intField(hit.Fields, "line_start")
		lineEnd := intField(hit.Fields, "line_end")
		line, snippet, excerpt := matchingLine(content, tokens, lineStart)
		results = append(results, SearchResult{
			Path:       stringField(hit.Fields, "path"),
			Score:      hit.Score,
			Line:       line,
			LineStart:  lineStart,
			LineEnd:    lineEnd,
			Snippet:    snippet,
			Excerpt:    excerpt,
			Provenance: stringField(hit.Fields, "provenance"),
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
	absPath   string
	relPath   string
	sizeBytes int64
	modTime   string
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

func discoverFiles(root string, maxFileSize int64, ignorePatterns []string) (discoverySummary, error) {
	summary := discoverySummary{
		skippedReasons: map[string]int{},
	}
	ignoreRules, err := ignoreRulesFromPatterns(ignorePatterns)
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

		summary.files = append(summary.files, discoveredFile{
			absPath:   path,
			relPath:   relPath,
			sizeBytes: info.Size(),
			modTime:   info.ModTime().UTC().Format(time.RFC3339Nano),
		})
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
	case ".go", ".mod", ".sum", ".md", ".txt", ".yaml", ".yml", ".json", ".toml", ".sql", ".sh", ".ts", ".tsx", ".js", ".jsx", ".vue", ".py", ".rs", ".http", ".css", ".html":
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

func loadGitignorePatterns(root string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read .gitignore: %w", err)
	}

	patterns, err := parseIgnorePatterns(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse .gitignore: %w", err)
	}
	return patterns, nil
}

func parseIgnorePatterns(input string) ([]string, error) {
	var patterns []string
	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, `\#`) {
			line = strings.TrimPrefix(line, `\`)
		}
		patterns = append(patterns, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return normalizePatterns(patterns), nil
}

func ignoreRulesFromPatterns(patterns []string) (ignoreRules, error) {
	var rules ignoreRules
	for _, line := range normalizePatterns(patterns) {
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

	return rules, nil
}

func normalizePatterns(patterns []string) []string {
	seen := make(map[string]struct{}, len(patterns))
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(filepath.ToSlash(pattern))
		if pattern == "" {
			continue
		}
		if _, ok := seen[pattern]; ok {
			continue
		}
		seen[pattern] = struct{}{}
		out = append(out, pattern)
	}
	return out
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
	matched, err := doublestar.Match(pattern, value)
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

func (s *Service) rebuildBleveIndex(projectID int64, chunks []indexedChunk) error {
	indexPath := s.projectIndexPath(projectID)
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o700); err != nil {
		return fmt.Errorf("create index directory: %w", err)
	}
	tempPath := indexPath + ".tmp"
	_ = os.RemoveAll(tempPath)

	index, err := bleve.New(tempPath, newIndexMapping())
	if err != nil {
		return fmt.Errorf("create project index: %w", err)
	}
	batch := index.NewBatch()
	for _, chunk := range chunks {
		if err := batch.Index(chunk.ID, chunk); err != nil {
			_ = index.Close()
			return fmt.Errorf("index chunk %q: %w", chunk.ID, err)
		}
	}
	if err := index.Batch(batch); err != nil {
		_ = index.Close()
		return fmt.Errorf("write project index batch: %w", err)
	}
	if err := index.Close(); err != nil {
		return fmt.Errorf("close project index: %w", err)
	}

	oldPath := indexPath + ".old"
	_ = os.RemoveAll(oldPath)
	if _, err := os.Stat(indexPath); err == nil {
		if err := os.Rename(indexPath, oldPath); err != nil {
			return fmt.Errorf("stage old project index: %w", err)
		}
	}
	if err := os.Rename(tempPath, indexPath); err != nil {
		_ = os.Rename(oldPath, indexPath)
		return fmt.Errorf("activate project index: %w", err)
	}
	_ = os.RemoveAll(oldPath)
	return nil
}

func newIndexMapping() *mapping.IndexMappingImpl {
	mapping := bleve.NewIndexMapping()
	mapping.DefaultAnalyzer = "standard"
	docMapping := bleve.NewDocumentMapping()
	for _, field := range []string{"path", "ext", "content", "provenance"} {
		fieldMapping := bleve.NewTextFieldMapping()
		fieldMapping.Store = true
		docMapping.AddFieldMappingsAt(field, fieldMapping)
	}
	searchFieldMapping := bleve.NewTextFieldMapping()
	searchFieldMapping.Store = false
	docMapping.AddFieldMappingsAt("search_content", searchFieldMapping)
	for _, field := range []string{"project_id", "line_start", "line_end"} {
		fieldMapping := bleve.NewNumericFieldMapping()
		fieldMapping.Store = true
		docMapping.AddFieldMappingsAt(field, fieldMapping)
	}
	mapping.DefaultMapping = docMapping
	return mapping
}

func (s *Service) projectIndexPath(projectID int64) string {
	root := s.indexRoot
	if root == "" {
		root = filepath.Join(os.TempDir(), "tok-indexes")
	}
	return filepath.Join(root, fmt.Sprintf("project-%d.bleve", projectID))
}

func chunkFile(projectID int64, relPath, content string) []indexedChunk {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	var chunks []indexedChunk
	for start := 0; start < len(lines); {
		end := start + chunkMaxLines
		if end > len(lines) {
			end = len(lines)
		}
		lineStart := start + 1
		lineEnd := end
		chunkContent := strings.Join(lines[start:end], "\n")
		chunks = append(chunks, indexedChunk{
			ID:         fmt.Sprintf("%s:%d-%d", relPath, lineStart, lineEnd),
			ProjectID:  projectID,
			Path:       relPath,
			Ext:        strings.TrimPrefix(strings.ToLower(filepath.Ext(relPath)), "."),
			Content:    chunkContent,
			SearchText: expandedSearchText(chunkContent),
			LineStart:  lineStart,
			LineEnd:    lineEnd,
			Provenance: "project_file",
		})
		if end == len(lines) {
			break
		}
		start = end - chunkOverlap
		if start < 0 {
			start = end
		}
	}
	return chunks
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func expandedSearchText(input string) string {
	var out strings.Builder
	out.WriteString(input)
	for _, match := range tokenPattern.FindAllString(input, -1) {
		for _, token := range expandToken(match) {
			out.WriteByte(' ')
			out.WriteString(token)
		}
	}
	return out.String()
}

func ensureProjectPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", os.ErrNotExist, path)
		}
		return fmt.Errorf("inspect project path %q: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("project path is not a directory: %s", path)
	}
	return nil
}

func stringField(fields map[string]any, key string) string {
	value, ok := fields[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func intField(fields map[string]any, key string) int {
	value, ok := fields[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		i, _ := typed.Int64()
		return int(i)
	default:
		i, _ := strconv.Atoi(fmt.Sprint(typed))
		return i
	}
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

func matchingLine(content string, tokens []string, baseLine int) (int, string, string) {
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
		absoluteLine := baseLine + bestLine - 1
		return absoluteLine, trimSnippet(lines[bestLine-1]), excerpt(lines, bestLine, 1, baseLine)
	}
	if len(lines) == 0 {
		return 0, "", ""
	}
	return baseLine, trimSnippet(lines[0]), excerpt(lines, 1, 1, baseLine)
}

func trimSnippet(line string) string {
	snippet := strings.Join(strings.Fields(line), " ")
	if len(snippet) <= 160 {
		return snippet
	}
	return snippet[:157] + "..."
}

func excerpt(lines []string, lineNumber, radius, baseLine int) string {
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
		fmt.Fprintf(&out, "%d: %s", baseLine+i-1, trimSnippet(lines[i-1]))
	}
	return out.String()
}
