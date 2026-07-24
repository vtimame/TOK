package app

import (
	"context"
	"fmt"
	"strings"

	"s26.sh/tok/internal/httpserver"
)

type uiServeOptions struct {
	addr string
}

func (c *CLI) runUI(ctx context.Context, opts runtimeOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(opts.args) < 2 {
		return &UsageError{
			Message: fmt.Sprintf("missing ui command\n\nRun '%s help' for usage.", commandName),
			Code:    2,
		}
	}

	switch opts.args[1] {
	case "serve":
		return c.runUIServe(ctx, opts)
	default:
		return &UsageError{
			Message: fmt.Sprintf("unknown ui command %q\n\nRun '%s help' for usage.", opts.args[1], commandName),
			Code:    2,
		}
	}
}

func (c *CLI) runUIServe(ctx context.Context, opts runtimeOptions) error {
	serveOpts, err := parseUIServeOptions(opts.args[2:])
	if err != nil {
		return err
	}

	_, _, store, err := c.runtimeStore(ctx, opts)
	if err != nil {
		return err
	}
	defer store.Close()

	server, err := httpserver.New(httpserver.Config{
		Addr:    serveOpts.addr,
		Store:   store,
		Version: c.version.Version,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(c.out, "serving TOK UI API on http://%s\n", serveOpts.addr)
	return server.Run(ctx)
}

func parseUIServeOptions(args []string) (uiServeOptions, error) {
	opts := uiServeOptions{addr: "127.0.0.1:7654"}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--addr":
			i++
			if i >= len(args) {
				return uiServeOptions{}, &UsageError{Message: "--addr requires a value", Code: 2}
			}
			opts.addr = args[i]
		case strings.HasPrefix(arg, "--addr="):
			opts.addr = strings.TrimPrefix(arg, "--addr=")
			if opts.addr == "" {
				return uiServeOptions{}, &UsageError{Message: "--addr requires a value", Code: 2}
			}
		default:
			return uiServeOptions{}, &UsageError{Message: fmt.Sprintf("unknown ui serve option %q", arg), Code: 2}
		}
	}

	opts.addr = strings.TrimSpace(opts.addr)
	if opts.addr == "" {
		return uiServeOptions{}, &UsageError{Message: "ui serve requires --addr", Code: 2}
	}

	return opts, nil
}
