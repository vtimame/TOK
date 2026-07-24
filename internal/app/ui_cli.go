package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	"s26.sh/tok/internal/httpserver"
	"s26.sh/tok/internal/storage"
)

type uiServeOptions struct {
	addr string
}

type uiOpenAPIOptions struct {
	out string
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
	case "openapi":
		return c.runUIOpenAPI(ctx, opts)
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

func (c *CLI) runUIOpenAPI(ctx context.Context, opts runtimeOptions) error {
	openAPIOpts, err := parseUIOpenAPIOptions(opts.args[2:])
	if err != nil {
		return err
	}

	store, err := storage.Open(ctx, ":memory:")
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Init(ctx); err != nil {
		return err
	}

	server, err := httpserver.New(httpserver.Config{
		Addr:    "127.0.0.1:0",
		Store:   store,
		Version: c.version.Version,
	})
	if err != nil {
		return err
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/swagger/openapi.json", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		return fmt.Errorf("render OpenAPI spec: status %d: %s", recorder.Code, strings.TrimSpace(recorder.Body.String()))
	}
	spec, err := sanitizeUIOpenAPISpec(recorder.Body.Bytes())
	if err != nil {
		return err
	}

	if openAPIOpts.out == "" {
		_, err := c.out.Write(spec)
		return err
	}

	dir := filepath.Dir(openAPIOpts.out)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(openAPIOpts.out, spec, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(c.out, "wrote OpenAPI spec: %s\n", openAPIOpts.out)
	return nil
}

func sanitizeUIOpenAPISpec(data []byte) ([]byte, error) {
	var spec map[string]any
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("decode OpenAPI spec: %w", err)
	}

	paths, _ := spec["paths"].(map[string]any)
	for _, pathItem := range paths {
		operations, _ := pathItem.(map[string]any)
		for _, operation := range operations {
			op, _ := operation.(map[string]any)
			params, ok := op["parameters"].([]any)
			if !ok {
				continue
			}

			filtered := params[:0]
			for _, param := range params {
				if !isAcceptHeaderParam(param) {
					filtered = append(filtered, param)
				}
			}
			if len(filtered) == 0 {
				delete(op, "parameters")
			} else {
				op["parameters"] = filtered
			}
		}
	}

	out, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode OpenAPI spec: %w", err)
	}
	return append(out, '\n'), nil
}

func isAcceptHeaderParam(param any) bool {
	p, _ := param.(map[string]any)
	location, _ := p["in"].(string)
	name, _ := p["name"].(string)
	return strings.EqualFold(location, "header") && strings.EqualFold(name, "accept")
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

func parseUIOpenAPIOptions(args []string) (uiOpenAPIOptions, error) {
	var opts uiOpenAPIOptions

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--out":
			i++
			if i >= len(args) {
				return uiOpenAPIOptions{}, &UsageError{Message: "--out requires a value", Code: 2}
			}
			opts.out = args[i]
		case strings.HasPrefix(arg, "--out="):
			opts.out = strings.TrimPrefix(arg, "--out=")
			if opts.out == "" {
				return uiOpenAPIOptions{}, &UsageError{Message: "--out requires a value", Code: 2}
			}
		default:
			return uiOpenAPIOptions{}, &UsageError{Message: fmt.Sprintf("unknown ui openapi option %q", arg), Code: 2}
		}
	}

	opts.out = strings.TrimSpace(opts.out)
	return opts, nil
}
