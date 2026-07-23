package app

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/rs/zerolog"

	"s26.sh/tok/internal/config"
	"s26.sh/tok/internal/logging"
	"s26.sh/tok/internal/storage"
)

const commandName = "tok"

// CLI owns the root command surface shared by process entrypoints and tests.
type CLI struct {
	out       io.Writer
	err       io.Writer
	version   VersionInfo
	loadCfg   func(string) (config.Config, error)
	newLogger func(io.Writer, config.LogConfig) (zerolog.Logger, error)
}

type VersionInfo struct {
	Version string
	Commit  string
	Date    string
}

type UsageError struct {
	Message string
	Code    int
}

func (e *UsageError) Error() string {
	return e.Message
}

func NewCLI(out, err io.Writer, version VersionInfo) *CLI {
	if version.Version == "" {
		version.Version = "dev"
	}
	if version.Commit == "" {
		version.Commit = "unknown"
	}
	if version.Date == "" {
		version.Date = "unknown"
	}

	return &CLI{
		out:       out,
		err:       err,
		version:   version,
		loadCfg:   config.LoadFromOS,
		newLogger: logging.NewLogger,
	}
}

func (c *CLI) Run(ctx context.Context, args []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	opts, err := parseRuntimeOptions(args)
	if err != nil {
		return err
	}

	if len(opts.args) == 0 {
		c.printHelp()
		return nil
	}

	switch opts.args[0] {
	case "help", "-h", "--help":
		c.printHelp()
		return nil
	case "version", "-v", "--version":
		c.printVersion()
		return nil
	case "init":
		return c.runInit(ctx, opts)
	case "config":
		return c.runConfig(ctx, opts)
	case "project":
		return c.runProject(ctx, opts)
	case "task":
		return c.runTask(ctx, opts)
	case "index":
		return c.runIndex(ctx, opts)
	case "search":
		return c.runSearch(ctx, opts)
	case "context":
		return c.runContext(ctx, opts)
	default:
		return &UsageError{
			Message: fmt.Sprintf("unknown command %q\n\nRun '%s help' for usage.", opts.args[0], commandName),
			Code:    2,
		}
	}
}

type runtimeOptions struct {
	configPath string
	logLevel   string
	args       []string
}

func parseRuntimeOptions(args []string) (runtimeOptions, error) {
	var opts runtimeOptions

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--config":
			i++
			if i >= len(args) {
				return runtimeOptions{}, &UsageError{Message: "--config requires a path", Code: 2}
			}
			opts.configPath = args[i]
		case strings.HasPrefix(arg, "--config="):
			opts.configPath = strings.TrimPrefix(arg, "--config=")
			if opts.configPath == "" {
				return runtimeOptions{}, &UsageError{Message: "--config requires a path", Code: 2}
			}
		case arg == "--log-level":
			i++
			if i >= len(args) {
				return runtimeOptions{}, &UsageError{Message: "--log-level requires a value", Code: 2}
			}
			opts.logLevel = args[i]
		case strings.HasPrefix(arg, "--log-level="):
			opts.logLevel = strings.TrimPrefix(arg, "--log-level=")
			if opts.logLevel == "" {
				return runtimeOptions{}, &UsageError{Message: "--log-level requires a value", Code: 2}
			}
		default:
			opts.args = args[i:]
			return opts, nil
		}
	}

	return opts, nil
}

func (c *CLI) runtime(opts runtimeOptions) (config.Config, zerolog.Logger, error) {
	cfg, err := c.loadCfg(opts.configPath)
	if err != nil {
		return config.Config{}, zerolog.Logger{}, err
	}
	if opts.logLevel != "" {
		cfg.Log.Level = opts.logLevel
	}

	logger, err := c.newLogger(c.err, cfg.Log)
	if err != nil {
		return config.Config{}, zerolog.Logger{}, err
	}

	return cfg, logger, nil
}

func (c *CLI) runInit(ctx context.Context, opts runtimeOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	cfg, logger, store, err := c.runtimeStore(ctx, opts)
	if err != nil {
		return err
	}
	defer store.Close()

	dbPath := storage.DatabasePath(cfg.DataDir)
	logger.Debug().Str("database", dbPath).Msg("initialized runtime database")
	fmt.Fprintf(c.out, "initialized database: %s\n", dbPath)
	return nil
}

func (c *CLI) runtimeStore(ctx context.Context, opts runtimeOptions) (config.Config, zerolog.Logger, *storage.Store, error) {
	cfg, logger, err := c.runtime(opts)
	if err != nil {
		return config.Config{}, zerolog.Logger{}, nil, err
	}

	store, err := storage.Open(ctx, storage.DatabasePath(cfg.DataDir))
	if err != nil {
		return config.Config{}, zerolog.Logger{}, nil, err
	}
	if err := store.Init(ctx); err != nil {
		_ = store.Close()
		return config.Config{}, zerolog.Logger{}, nil, err
	}

	return cfg, logger, store, nil
}

func (c *CLI) runConfig(ctx context.Context, opts runtimeOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(opts.args) < 2 {
		return &UsageError{
			Message: fmt.Sprintf("missing config command\n\nRun '%s help' for usage.", commandName),
			Code:    2,
		}
	}

	switch opts.args[1] {
	case "paths":
		cfg, logger, err := c.runtime(opts)
		if err != nil {
			return err
		}
		logger.Debug().Str("data_dir", cfg.DataDir).Msg("resolved runtime paths")
		fmt.Fprintf(c.out, "data_dir: %s\n", cfg.DataDir)
		return nil
	default:
		return &UsageError{
			Message: fmt.Sprintf("unknown config command %q\n\nRun '%s help' for usage.", opts.args[1], commandName),
			Code:    2,
		}
	}
}

func (c *CLI) printHelp() {
	fmt.Fprint(c.out, strings.TrimSpace(helpText)+"\n")
}

func (c *CLI) printVersion() {
	fmt.Fprintf(c.out, "%s %s\ncommit: %s\nbuilt: %s\n", commandName, c.version.Version, c.version.Commit, c.version.Date)
}

const helpText = `
TOK - Task Operations Kernel

Usage:
  tok [--config <path>] [--log-level <level>] <command>

Commands:
  version   Print build version information
  init      Initialize local runtime storage
  config    Inspect runtime configuration
  project   Register and inspect local projects
  task      Create and update project tasks
  index     Update local retrieval indexes
  search    Search indexed project files
  context   Build compact task context packages
  help      Show this help
`
