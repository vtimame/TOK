package retrieval

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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
}

type Service struct {
	store       Store
	maxFileSize int64
}

type IndexSummary struct {
	ProjectName      string
	IndexedDocuments int
	SkippedFiles     int
}

type SearchResult struct {
	Path       string
	Score      float64
	Line       int
	Snippet    string
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

	files, skipped, err := discoverFiles(project.Path, s.maxFileSize)
	if err != nil {
		return IndexSummary{}, err
	}

	docs := make([]storage.IndexedDocumentInput, 0, len(files))
	for _, file := range files {
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

	return IndexSummary{
		ProjectName:      project.Name,
		IndexedDocuments: indexed,
		SkippedFiles:     skipped,
	}, nil
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
		line, snippet := matchingLine(rankedDoc.Content, tokens)
		results = append(results, SearchResult{
			Path:       rankedDoc.Path,
			Score:      rankedDoc.Score,
			Line:       line,
			Snippet:    snippet,
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

func discoverFiles(root string, maxFileSize int64) ([]discoveredFile, int, error) {
	var files []discoveredFile
	var skipped int

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}

		name := entry.Name()
		if entry.IsDir() {
			if shouldSkipDir(name) {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)

		if !shouldIndexFile(relPath, name) {
			skipped++
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxFileSize {
			skipped++
			return nil
		}

		text, err := looksTextFile(path)
		if err != nil {
			return err
		}
		if !text {
			skipped++
			return nil
		}

		files = append(files, discoveredFile{absPath: path, relPath: relPath})
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("discover project files: %w", err)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].relPath < files[j].relPath
	})

	return files, skipped, nil
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

func matchingLine(content string, tokens []string) (int, string) {
	lines := strings.Split(content, "\n")
	for idx, line := range lines {
		lower := strings.ToLower(line)
		for _, token := range tokens {
			if strings.Contains(lower, token) {
				return idx + 1, trimSnippet(line)
			}
		}
	}
	if len(lines) == 0 {
		return 0, ""
	}
	return 1, trimSnippet(lines[0])
}

func trimSnippet(line string) string {
	snippet := strings.Join(strings.Fields(line), " ")
	if len(snippet) <= 160 {
		return snippet
	}
	return snippet[:157] + "..."
}
