package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const agentTokenPrefix = "tok_agent_"

type Actor struct {
	ID             int64
	Kind           string
	Name           string
	TokenHash      string
	TokenRevokedAt string
	CreatedAt      string
	UpdatedAt      string
}

type AgentWithToken struct {
	Agent Actor
	Token string
}

type AgentActivity struct {
	Actor          Actor
	TasksCount     int
	EventsCount    int
	LastActivityAt string
}

type AgentProjectActivity struct {
	ActorID        int64
	ProjectID      int64
	ProjectName    string
	ProjectDisplay string
	TasksCount     int
	EventsCount    int
	LastActivityAt string
}

type ActorRef struct {
	ID   int64
	Kind string
	Name string
}

type UpdateAgentInput struct {
	Name string
}

func ActorRefFromActor(actor Actor) ActorRef {
	return ActorRef{
		ID:   actor.ID,
		Kind: actor.Kind,
		Name: actor.Name,
	}
}

func (s *Store) SetLocalHuman(ctx context.Context, displayName string) (Actor, error) {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return Actor{}, errors.New("user display name is required")
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO actors (kind, name)
		VALUES ('human', ?)
		ON CONFLICT(kind) WHERE kind = 'human' DO UPDATE SET
			name = excluded.name,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
	`, displayName)
	if err != nil {
		return Actor{}, fmt.Errorf("set local user: %w", err)
	}

	return s.GetLocalHuman(ctx)
}

func (s *Store) GetLocalHuman(ctx context.Context) (Actor, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, kind, name, token_hash, token_revoked_at, created_at, updated_at
		FROM actors
		WHERE kind = 'human'
	`)
	return scanActor(row)
}

func (s *Store) CreateAgent(ctx context.Context, name string) (AgentWithToken, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return AgentWithToken{}, errors.New("agent name is required")
	}

	token, err := newAgentToken()
	if err != nil {
		return AgentWithToken{}, err
	}
	tokenHash := hashAgentToken(token)

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO actors (kind, name, token_hash)
		VALUES ('agent', ?, ?)
	`, name, tokenHash)
	if err != nil {
		return AgentWithToken{}, fmt.Errorf("create agent %q: %w", name, err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return AgentWithToken{}, fmt.Errorf("read created agent id: %w", err)
	}

	agent, err := s.GetActor(ctx, id)
	if err != nil {
		return AgentWithToken{}, err
	}

	return AgentWithToken{Agent: agent, Token: token}, nil
}

func (s *Store) GetActor(ctx context.Context, id int64) (Actor, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, kind, name, token_hash, token_revoked_at, created_at, updated_at
		FROM actors
		WHERE id = ?
	`, id)
	return scanActor(row)
}

func (s *Store) ListAgents(ctx context.Context) ([]Actor, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, kind, name, token_hash, token_revoked_at, created_at, updated_at
		FROM actors
		WHERE kind = 'agent'
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var agents []Actor
	for rows.Next() {
		agent, err := scanActor(rows)
		if err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agents: %w", err)
	}

	return agents, nil
}

func (s *Store) UpdateAgent(ctx context.Context, id int64, input UpdateAgentInput) (Actor, error) {
	if id <= 0 {
		return Actor{}, errors.New("agent id is required")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Actor{}, errors.New("agent name is required")
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE actors
		SET name = ?,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ? AND kind = 'agent'
	`, name, id)
	if err != nil {
		return Actor{}, fmt.Errorf("update agent %d: %w", id, err)
	}
	updated, err := res.RowsAffected()
	if err != nil {
		return Actor{}, fmt.Errorf("read updated agent count: %w", err)
	}
	if updated == 0 {
		return Actor{}, sql.ErrNoRows
	}

	return s.GetActor(ctx, id)
}

func (s *Store) DeleteAgent(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("agent id is required")
	}
	res, err := s.db.ExecContext(ctx, "DELETE FROM actors WHERE id = ? AND kind = 'agent'", id)
	if err != nil {
		return fmt.Errorf("delete agent %d: %w", id, err)
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted agent count: %w", err)
	}
	if deleted == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListAgentActivity(ctx context.Context) ([]AgentActivity, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			actors.id,
			actors.kind,
			actors.name,
			actors.token_hash,
			actors.token_revoked_at,
			actors.created_at,
			actors.updated_at,
			COALESCE(activity.tasks_count, 0),
			COALESCE(activity.events_count, 0),
			COALESCE(activity.last_activity_at, '')
		FROM actors
		LEFT JOIN (
			SELECT
				actor_id,
				COUNT(DISTINCT task_id) AS tasks_count,
				COUNT(*) AS events_count,
				MAX(created_at) AS last_activity_at
			FROM task_events
			WHERE actor_kind = 'agent' AND actor_id > 0
			GROUP BY actor_id
		) activity ON activity.actor_id = actors.id
		WHERE actors.kind = 'agent'
		ORDER BY
			CASE WHEN activity.last_activity_at IS NULL THEN 1 ELSE 0 END,
			activity.last_activity_at DESC,
			actors.name
	`)
	if err != nil {
		return nil, fmt.Errorf("list agent activity: %w", err)
	}
	defer rows.Close()

	var agents []AgentActivity
	for rows.Next() {
		var agent AgentActivity
		if err := rows.Scan(
			&agent.Actor.ID,
			&agent.Actor.Kind,
			&agent.Actor.Name,
			&agent.Actor.TokenHash,
			&agent.Actor.TokenRevokedAt,
			&agent.Actor.CreatedAt,
			&agent.Actor.UpdatedAt,
			&agent.TasksCount,
			&agent.EventsCount,
			&agent.LastActivityAt,
		); err != nil {
			return nil, fmt.Errorf("scan agent activity: %w", err)
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent activity: %w", err)
	}

	return agents, nil
}

func (s *Store) ListAgentProjectActivity(ctx context.Context) ([]AgentProjectActivity, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			task_events.actor_id,
			projects.id,
			projects.name,
			projects.display_name,
			COUNT(DISTINCT task_events.task_id) AS tasks_count,
			COUNT(*) AS events_count,
			MAX(task_events.created_at) AS last_activity_at
		FROM task_events
		JOIN actors ON actors.id = task_events.actor_id AND actors.kind = 'agent'
		JOIN tasks ON tasks.id = task_events.task_id
		JOIN projects ON projects.id = tasks.project_id
		WHERE task_events.actor_kind = 'agent' AND task_events.actor_id > 0
		GROUP BY task_events.actor_id, projects.id
		ORDER BY projects.display_name, projects.id
	`)
	if err != nil {
		return nil, fmt.Errorf("list agent project activity: %w", err)
	}
	defer rows.Close()

	var projects []AgentProjectActivity
	for rows.Next() {
		var project AgentProjectActivity
		if err := rows.Scan(
			&project.ActorID,
			&project.ProjectID,
			&project.ProjectName,
			&project.ProjectDisplay,
			&project.TasksCount,
			&project.EventsCount,
			&project.LastActivityAt,
		); err != nil {
			return nil, fmt.Errorf("scan agent project activity: %w", err)
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent project activity: %w", err)
	}

	return projects, nil
}

func (s *Store) RevokeAgent(ctx context.Context, id int64) (Actor, error) {
	if id <= 0 {
		return Actor{}, errors.New("agent id is required")
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE actors
		SET token_revoked_at = CASE
				WHEN token_revoked_at = '' THEN strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
				ELSE token_revoked_at
			END,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ? AND kind = 'agent'
	`, id)
	if err != nil {
		return Actor{}, fmt.Errorf("revoke agent: %w", err)
	}
	updated, err := res.RowsAffected()
	if err != nil {
		return Actor{}, fmt.Errorf("read revoked agent count: %w", err)
	}
	if updated == 0 {
		return Actor{}, sql.ErrNoRows
	}

	return s.GetActor(ctx, id)
}

func (s *Store) ResolveAgentByToken(ctx context.Context, token string) (Actor, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Actor{}, errors.New("agent token is required")
	}
	tokenHash := hashAgentToken(token)

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, kind, name, token_hash, token_revoked_at, created_at, updated_at
		FROM actors
		WHERE kind = 'agent' AND token_revoked_at = ''
	`)
	if err != nil {
		return Actor{}, fmt.Errorf("resolve agent token: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		agent, err := scanActor(rows)
		if err != nil {
			return Actor{}, fmt.Errorf("scan agent token candidate: %w", err)
		}
		if subtle.ConstantTimeCompare([]byte(agent.TokenHash), []byte(tokenHash)) == 1 {
			return agent, nil
		}
	}
	if err := rows.Err(); err != nil {
		return Actor{}, fmt.Errorf("iterate agent token candidates: %w", err)
	}

	return Actor{}, sql.ErrNoRows
}

func scanActor(row scanner) (Actor, error) {
	var actor Actor
	if err := row.Scan(&actor.ID, &actor.Kind, &actor.Name, &actor.TokenHash, &actor.TokenRevokedAt, &actor.CreatedAt, &actor.UpdatedAt); err != nil {
		return Actor{}, err
	}
	return actor, nil
}

func newAgentToken() (string, error) {
	var data [32]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("generate agent token: %w", err)
	}
	return agentTokenPrefix + base64.RawURLEncoding.EncodeToString(data[:]), nil
}

func hashAgentToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return "sha256:" + hex.EncodeToString(sum[:])
}
