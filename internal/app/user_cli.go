package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"strings"

	"s26.sh/tok/internal/storage"
)

type resolvedUserDisplayName struct {
	Name   string
	Source string
}

type userOptions struct {
	name string
	json bool
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
	jsonOutput, err := parseNoArgJSONOption(args, "user show")
	if err != nil {
		return err
	}

	resolved, err := resolveLocalUserDisplayName(ctx, store)
	if err != nil {
		return err
	}

	if jsonOutput {
		return printResolvedUserJSON(c.out, resolved)
	}
	fmt.Fprintf(c.out, "display_name: %s\n", resolved.Name)
	fmt.Fprintf(c.out, "source: %s\n", resolved.Source)
	return nil
}

func (c *CLI) runUserSetName(ctx context.Context, store *storage.Store, args []string) error {
	setOpts, err := parseUserSetNameOptions(args)
	if err != nil {
		return err
	}

	actor, err := store.SetLocalHuman(ctx, setOpts.name)
	if err != nil {
		return err
	}

	if setOpts.json {
		return printUserActorJSON(c.out, actor)
	}
	fmt.Fprintf(c.out, "id: %d\n", actor.ID)
	fmt.Fprintf(c.out, "kind: %s\n", actor.Kind)
	fmt.Fprintf(c.out, "display_name: %s\n", actor.Name)
	fmt.Fprintf(c.out, "created_at: %s\n", actor.CreatedAt)
	fmt.Fprintf(c.out, "updated_at: %s\n", actor.UpdatedAt)
	return nil
}

func parseUserSetNameOptions(args []string) (userOptions, error) {
	var opts userOptions
	for _, arg := range args {
		switch {
		case arg == "--json":
			opts.json = true
		case strings.HasPrefix(arg, "-"):
			return userOptions{}, &UsageError{Message: fmt.Sprintf("unknown user set-name option %q", arg), Code: 2}
		default:
			if opts.name != "" {
				return userOptions{}, &UsageError{Message: "user set-name accepts exactly one display name", Code: 2}
			}
			opts.name = strings.TrimSpace(arg)
		}
	}
	if opts.name == "" {
		return userOptions{}, &UsageError{Message: "user set-name requires a display name", Code: 2}
	}
	return opts, nil
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

type resolvedUserOutput struct {
	DisplayName string `json:"display_name"`
	Source      string `json:"source"`
}

type userActorOutput struct {
	ID          int64  `json:"id"`
	Kind        string `json:"kind"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func printResolvedUserJSON(out io.Writer, resolved resolvedUserDisplayName) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(resolvedUserOutput{
		DisplayName: resolved.Name,
		Source:      resolved.Source,
	})
}

func printUserActorJSON(out io.Writer, actor storage.Actor) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(userActorOutput{
		ID:          actor.ID,
		Kind:        actor.Kind,
		DisplayName: actor.Name,
		CreatedAt:   actor.CreatedAt,
		UpdatedAt:   actor.UpdatedAt,
	})
}
