package indexwatch

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"s26.sh/tok/internal/retrieval"
	"s26.sh/tok/internal/storage"
)

const (
	DefaultDebounce         = 3 * time.Second
	DefaultRegistryInterval = 10 * time.Second
)

type EventType string

const (
	EventProjectWatching EventType = "project_watching"
	EventProjectRemoved  EventType = "project_removed"
	EventPathMissing     EventType = "path_missing"
	EventPathRestored    EventType = "path_restored"
	EventIndexQueued     EventType = "index_queued"
	EventIndexStarted    EventType = "index_started"
	EventIndexCompleted  EventType = "index_completed"
	EventIndexFailed     EventType = "index_failed"
	EventWatchError      EventType = "watch_error"
)

type Store interface {
	ListProjects(ctx context.Context) ([]storage.Project, error)
	GetProjectByID(ctx context.Context, id int64) (storage.Project, error)
}

type Indexer interface {
	IndexProject(ctx context.Context, project storage.Project) (retrieval.IndexSummary, error)
	IgnorePolicy(ctx context.Context, project storage.Project) (retrieval.IndexPolicy, error)
}

type Config struct {
	Store            Store
	Indexer          Indexer
	Debounce         time.Duration
	RegistryInterval time.Duration
	ProjectName      string
	NoInitialIndex   bool
	Events           chan<- Event
}

type Event struct {
	Type        EventType
	ProjectName string
	Path        string
	Message     string
	Summary     *retrieval.IndexSummary
	Err         error
}

type Service struct {
	store            Store
	indexer          Indexer
	debounce         time.Duration
	registryInterval time.Duration
	projectName      string
	noInitialIndex   bool
	events           chan<- Event

	fileEvents chan fileEvent
	jobs       chan int64

	mu          sync.Mutex
	generations map[int64]int64
	queued      map[int64]bool
}

type fileEvent struct {
	projectID int64
	event     fsnotify.Event
	err       error
}

type projectWatch struct {
	project        storage.Project
	watcher        *fsnotify.Watcher
	watchedDirs    map[string]struct{}
	ignorePatterns []string
}

func New(config Config) (*Service, error) {
	if config.Store == nil {
		return nil, errors.New("index watch store is required")
	}
	if config.Indexer == nil {
		return nil, errors.New("index watch indexer is required")
	}
	if config.Debounce <= 0 {
		config.Debounce = DefaultDebounce
	}
	if config.RegistryInterval <= 0 {
		config.RegistryInterval = DefaultRegistryInterval
	}
	return &Service{
		store:            config.Store,
		indexer:          config.Indexer,
		debounce:         config.Debounce,
		registryInterval: config.RegistryInterval,
		projectName:      strings.TrimSpace(config.ProjectName),
		noInitialIndex:   config.NoInitialIndex,
		events:           config.Events,
		fileEvents:       make(chan fileEvent, 256),
		jobs:             make(chan int64, 64),
		generations:      map[int64]int64{},
		queued:           map[int64]bool{},
	}, nil
}

func (s *Service) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	watches := map[int64]*projectWatch{}
	missingPaths := map[int64]string{}

	defer func() {
		for _, watch := range watches {
			_ = watch.watcher.Close()
		}
	}()

	workerDone := make(chan struct{})
	go s.runWorker(ctx, workerDone)

	if err := s.syncProjects(ctx, watches, missingPaths); err != nil {
		return err
	}

	ticker := time.NewTicker(s.registryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			<-workerDone
			return ctx.Err()
		case <-ticker.C:
			if err := s.syncProjects(ctx, watches, missingPaths); err != nil {
				return err
			}
		case event := <-s.fileEvents:
			if event.err != nil {
				if watch := watches[event.projectID]; watch != nil {
					s.emit(Event{
						Type:        EventWatchError,
						ProjectName: watch.project.Name,
						Path:        watch.project.Path,
						Err:         event.err,
					})
				}
				continue
			}
			watch := watches[event.projectID]
			if watch == nil || !watch.shouldHandle(event.event.Name) {
				continue
			}
			if event.event.Has(fsnotify.Create) {
				_ = watch.addCreatedDirectory(event.event.Name)
			}
			s.scheduleIndex(event.projectID, watch.project.Name, event.event.Name, "filesystem change")
		}
	}
}

func (s *Service) syncProjects(ctx context.Context, watches map[int64]*projectWatch, missingPaths map[int64]string) error {
	projects, err := s.store.ListProjects(ctx)
	if err != nil {
		return err
	}

	seen := map[int64]struct{}{}
	for _, project := range projects {
		if s.projectName != "" && project.Name != s.projectName {
			continue
		}
		seen[project.ID] = struct{}{}

		if err := ensureWatchablePath(project.Path); err != nil {
			if watch := watches[project.ID]; watch != nil {
				_ = watch.watcher.Close()
				delete(watches, project.ID)
			}
			if missingPaths[project.ID] != project.Path {
				missingPaths[project.ID] = project.Path
				s.emit(Event{
					Type:        EventPathMissing,
					ProjectName: project.Name,
					Path:        project.Path,
					Err:         err,
				})
				if !s.noInitialIndex {
					s.scheduleIndex(project.ID, project.Name, project.Path, "path missing")
				}
			}
			continue
		}

		if _, wasMissing := missingPaths[project.ID]; wasMissing {
			delete(missingPaths, project.ID)
			s.emit(Event{Type: EventPathRestored, ProjectName: project.Name, Path: project.Path})
			if !s.noInitialIndex {
				s.scheduleIndex(project.ID, project.Name, project.Path, "path restored")
			}
		}

		policy, err := s.indexer.IgnorePolicy(ctx, project)
		if err != nil {
			return err
		}

		existing := watches[project.ID]
		if existing != nil && existing.project.Path == project.Path {
			existing.project = project
			existing.ignorePatterns = policy.IgnorePatterns
			continue
		}
		if existing != nil {
			_ = existing.watcher.Close()
			delete(watches, project.ID)
		}

		watch, err := newProjectWatch(project, policy.IgnorePatterns, s.fileEvents)
		if err != nil {
			s.emit(Event{
				Type:        EventWatchError,
				ProjectName: project.Name,
				Path:        project.Path,
				Err:         err,
			})
			continue
		}
		watches[project.ID] = watch
		s.emit(Event{Type: EventProjectWatching, ProjectName: project.Name, Path: project.Path})
		if !s.noInitialIndex {
			s.scheduleIndex(project.ID, project.Name, project.Path, "initial sync")
		}
	}

	for projectID, watch := range watches {
		if _, ok := seen[projectID]; ok {
			continue
		}
		_ = watch.watcher.Close()
		delete(watches, projectID)
		s.emit(Event{Type: EventProjectRemoved, ProjectName: watch.project.Name, Path: watch.project.Path})
	}
	for projectID := range missingPaths {
		if _, ok := seen[projectID]; !ok {
			delete(missingPaths, projectID)
		}
	}

	return nil
}

func (s *Service) scheduleIndex(projectID int64, projectName, path, reason string) {
	s.mu.Lock()
	s.generations[projectID]++
	generation := s.generations[projectID]
	s.mu.Unlock()

	s.emit(Event{
		Type:        EventIndexQueued,
		ProjectName: projectName,
		Path:        path,
		Message:     reason,
	})

	go func() {
		timer := time.NewTimer(s.debounce)
		defer timer.Stop()

		<-timer.C
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.generations[projectID] != generation || s.queued[projectID] {
			return
		}
		s.queued[projectID] = true
		select {
		case s.jobs <- projectID:
		default:
			s.queued[projectID] = false
			s.emit(Event{
				Type:        EventWatchError,
				ProjectName: projectName,
				Path:        path,
				Err:         errors.New("index watch queue is full"),
			})
		}
	}()
}

func (s *Service) runWorker(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case <-ctx.Done():
			return
		case projectID := <-s.jobs:
			s.mu.Lock()
			s.queued[projectID] = false
			s.mu.Unlock()

			project, err := s.store.GetProjectByID(ctx, projectID)
			if err != nil {
				if !errors.Is(err, sql.ErrNoRows) {
					s.emit(Event{Type: EventIndexFailed, Err: err})
				}
				continue
			}
			s.emit(Event{Type: EventIndexStarted, ProjectName: project.Name, Path: project.Path})
			summary, err := s.indexer.IndexProject(ctx, project)
			if err != nil {
				s.emit(Event{Type: EventIndexFailed, ProjectName: project.Name, Path: project.Path, Err: err})
				continue
			}
			if summary.State == retrieval.StateFailed {
				s.emit(Event{Type: EventIndexFailed, ProjectName: project.Name, Path: project.Path, Summary: &summary, Err: errors.New(summary.LastError)})
				continue
			}
			s.emit(Event{Type: EventIndexCompleted, ProjectName: project.Name, Path: project.Path, Summary: &summary})
		}
	}
}

func newProjectWatch(project storage.Project, ignorePatterns []string, out chan<- fileEvent) (*projectWatch, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	watch := &projectWatch{
		project:        project,
		watcher:        watcher,
		watchedDirs:    map[string]struct{}{},
		ignorePatterns: append([]string{}, ignorePatterns...),
	}
	if err := watch.addDirectoryTree(project.Path); err != nil {
		_ = watcher.Close()
		return nil, err
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				select {
				case out <- fileEvent{projectID: project.ID, event: event}:
				default:
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				select {
				case out <- fileEvent{projectID: project.ID, err: err}:
				default:
				}
			}
		}
	}()

	return watch, nil
}

func (w *projectWatch) addCreatedDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return err
	}
	return w.addDirectoryTree(path)
}

func (w *projectWatch) addDirectoryTree(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if path != w.project.Path {
			if retrieval.ShouldSkipDirectoryName(entry.Name()) {
				return filepath.SkipDir
			}
			if w.pathIgnored(path, true) {
				return filepath.SkipDir
			}
		}
		if _, ok := w.watchedDirs[path]; ok {
			return nil
		}
		if err := w.watcher.Add(path); err != nil {
			return fmt.Errorf("watch directory %q: %w", path, err)
		}
		w.watchedDirs[path] = struct{}{}
		return nil
	})
}

func (w *projectWatch) shouldHandle(path string) bool {
	if path == "" {
		return false
	}
	if isNestedInSkippedDir(w.project.Path, path) {
		return false
	}
	info, err := os.Stat(path)
	isDir := err == nil && info.IsDir()
	return !w.pathIgnored(path, isDir)
}

func (w *projectWatch) pathIgnored(path string, isDir bool) bool {
	relPath, err := filepath.Rel(w.project.Path, path)
	if err != nil || relPath == "." || strings.HasPrefix(relPath, "..") {
		return false
	}
	ignored, err := retrieval.PathIgnoredByPatterns(w.ignorePatterns, filepath.ToSlash(relPath), isDir)
	return err == nil && ignored
}

func isNestedInSkippedDir(root, path string) bool {
	relPath, err := filepath.Rel(root, path)
	if err != nil || relPath == "." || strings.HasPrefix(relPath, "..") {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(relPath), "/") {
		if retrieval.ShouldSkipDirectoryName(part) {
			return true
		}
	}
	return false
}

func ensureWatchablePath(path string) error {
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

func (s *Service) emit(event Event) {
	if s.events == nil {
		return
	}
	select {
	case s.events <- event:
	default:
	}
}
