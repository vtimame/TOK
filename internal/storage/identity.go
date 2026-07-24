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

type ActorRef struct {
	ID   int64
	Kind string
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
