package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/user"
	"strings"

	"s26.sh/tok/internal/storage"
)

type resolvedUserDisplayName struct {
	Name   string
	Source string
}

func (c *CLI) runUser(ctx context.Context, opts runtimeOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(opts.args) < 2 {
		return &UsageError{
			Message: fmt.Sprintf("missing user command\n\nRun '%s help' for usage.", commandName),
			Code:    2,
		}
	}

	_, _, store, err := c.runtimeStore(ctx, opts)
	if err != nil {
		return err
	}
	defer store.Close()

	switch opts.args[1] {
	case "show":
		return c.runUserShow(ctx, store, opts.args[2:])
	case "set-name":
		return c.runUserSetName(ctx, store, opts.args[2:])
	default:
		return &UsageError{
			Message: fmt.Sprintf("unknown user command %q\n\nRun '%s help' for usage.", opts.args[1], commandName),
			Code:    2,
		}
	}
}

func (c *CLI) runUserShow(ctx context.Context, store *storage.Store, args []string) error {
	if len(args) > 0 {
		return &UsageError{Message: "user show does not accept arguments", Code: 2}
	}

	resolved, err := resolveLocalUserDisplayName(ctx, store)
	if err != nil {
		return err
	}

	fmt.Fprintf(c.out, "display_name: %s\n", resolved.Name)
	fmt.Fprintf(c.out, "source: %s\n", resolved.Source)
	return nil
}

func (c *CLI) runUserSetName(ctx context.Context, store *storage.Store, args []string) error {
	if len(args) != 1 {
		return &UsageError{Message: "user set-name requires a display name", Code: 2}
	}

	actor, err := store.SetLocalHuman(ctx, args[0])
	if err != nil {
		return err
	}

	fmt.Fprintf(c.out, "id: %d\n", actor.ID)
	fmt.Fprintf(c.out, "kind: %s\n", actor.Kind)
	fmt.Fprintf(c.out, "display_name: %s\n", actor.Name)
	fmt.Fprintf(c.out, "created_at: %s\n", actor.CreatedAt)
	fmt.Fprintf(c.out, "updated_at: %s\n", actor.UpdatedAt)
	return nil
}

func resolveLocalUserDisplayName(ctx context.Context, store *storage.Store) (resolvedUserDisplayName, error) {
	actor, err := store.GetLocalHuman(ctx)
	if err == nil {
		return resolvedUserDisplayName{Name: actor.Name, Source: "explicit"}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return resolvedUserDisplayName{}, err
	}

	if current, err := user.Current(); err == nil && current != nil {
		if name := strings.TrimSpace(current.Name); name != "" {
			return resolvedUserDisplayName{Name: name, Source: "system_user_name"}, nil
		}
		if username := strings.TrimSpace(current.Username); username != "" {
			return resolvedUserDisplayName{Name: username, Source: "system_username"}, nil
		}
	}

	for _, key := range []string{"USER", "USERNAME", "LOGNAME"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return resolvedUserDisplayName{Name: value, Source: "environment_" + strings.ToLower(key)}, nil
		}
	}

	return resolvedUserDisplayName{Name: "local-user", Source: "fallback"}, nil
}

func currentLocalHumanActor(ctx context.Context, store *storage.Store) (storage.ActorRef, error) {
	resolved, err := resolveLocalUserDisplayName(ctx, store)
	if err != nil {
		return storage.ActorRef{}, err
	}
	actor, err := store.SetLocalHuman(ctx, resolved.Name)
	if err != nil {
		return storage.ActorRef{}, err
	}
	return storage.ActorRefFromActor(actor), nil
}
