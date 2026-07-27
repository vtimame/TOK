package storage

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

func queryPlaceholders(count int) string {
	if count <= 0 {
		return ""
	}
	parts := make([]string, count)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
}

func normalizeTaskStatuses(opts ListTasksOptions) ([]string, error) {
	raw := opts.Statuses
	if len(raw) == 0 && strings.TrimSpace(opts.Status) != "" {
		raw = []string{opts.Status}
	}
	statuses := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, item := range raw {
		status := strings.TrimSpace(item)
		if status == "" || seen[status] {
			continue
		}
		if !validTaskStatus(status) {
			return nil, fmt.Errorf("invalid task status %q", status)
		}
		seen[status] = true
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func normalizeRunStatuses(opts ListRunsOptions) ([]string, error) {
	raw := opts.Statuses
	if len(raw) == 0 && strings.TrimSpace(opts.Status) != "" {
		raw = []string{opts.Status}
	}
	statuses := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, item := range raw {
		status := strings.TrimSpace(item)
		if status == "" || seen[status] {
			continue
		}
		if !validRunStatus(status) {
			return nil, fmt.Errorf("invalid run status %q", status)
		}
		seen[status] = true
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func validTaskStatus(status string) bool {
	switch status {
	case "open", "in_progress", "blocked", "done":
		return true
	default:
		return false
	}
}

func validRunStatus(status string) bool {
	switch status {
	case "created", "in_progress", "succeeded", "failed", "blocked", "cancelled":
		return true
	default:
		return false
	}
}

func runStatusTerminal(status string) bool {
	switch status {
	case "succeeded", "failed", "blocked", "cancelled":
		return true
	default:
		return false
	}
}

func validRunArtifactKind(kind string) bool {
	switch kind {
	case "handoff", "validation", "stdout", "stderr", "log", "patch", "note":
		return true
	default:
		return false
	}
}

func sanitizeActorRef(actor ActorRef) ActorRef {
	actor.Kind = strings.TrimSpace(actor.Kind)
	actor.Name = strings.TrimSpace(actor.Name)
	if actor.ID <= 0 || actor.Kind == "" || actor.Name == "" || !validActorKind(actor.Kind) {
		return ActorRef{}
	}
	return actor
}

func validActorKind(kind string) bool {
	switch kind {
	case "human", "agent", "system":
		return true
	default:
		return false
	}
}

func rollback(tx *sql.Tx) {
	if tx != nil {
		_ = tx.Rollback()
	}
}

func sqliteDSN(path string) string {
	if path == ":memory:" {
		return "file::memory:?cache=shared&_foreign_keys=on&_busy_timeout=5000"
	}
	values := url.Values{}
	values.Set("_foreign_keys", "on")
	values.Set("_busy_timeout", "5000")
	return (&url.URL{
		Scheme:   "file",
		Path:     filepath.ToSlash(path),
		RawQuery: values.Encode(),
	}).String()
}
