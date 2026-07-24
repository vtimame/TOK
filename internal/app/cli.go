package app

import (
	"context"
	"encoding/json"
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
	if helpPath, ok, err := helpRequestPath(opts.args); ok {
		if err != nil {
			return err
		}
		return c.printCommandHelp(helpPath)
	}

	switch opts.args[0] {
	case "help", "-h", "--help":
		c.printHelp()
		return nil
	case "version", "-v", "--version":
		return c.runVersion(opts.args[1:])
	case "init":
		return c.runInit(ctx, opts)
	case "config":
		return c.runConfig(ctx, opts)
	case "project":
		return c.runProject(ctx, opts)
	case "task":
		return c.runTask(ctx, opts)
	case "user":
		return c.runUser(ctx, opts)
	case "agent":
		return c.runAgent(ctx, opts)
	case "mcp":
		return c.runMCP(ctx, opts)
	case "ui":
		return c.runUI(ctx, opts)
	case "index":
		return c.runIndex(ctx, opts)
	case "search":
		return c.runSearch(ctx, opts)
	case "context":
		return c.runContext(ctx, opts)
	case "run":
		return c.runRun(ctx, opts)
	case "completion":
		return c.runCompletion(ctx, opts)
	case "__complete":
		return c.runComplete(ctx, opts)
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
	jsonOutput, err := parseNoArgJSONOption(opts.args[1:], "init")
	if err != nil {
		return err
	}

	cfg, logger, store, err := c.runtimeStore(ctx, opts)
	if err != nil {
		return err
	}
	defer store.Close()

	dbPath := storage.DatabasePath(cfg.DataDir)
	logger.Debug().Str("database", dbPath).Msg("initialized runtime database")
	if jsonOutput {
		return printInitJSON(c.out, cfg.DataDir, dbPath)
	}
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
		jsonOutput, err := parseNoArgJSONOption(opts.args[2:], "config paths")
		if err != nil {
			return err
		}
		cfg, logger, err := c.runtime(opts)
		if err != nil {
			return err
		}
		logger.Debug().Str("data_dir", cfg.DataDir).Msg("resolved runtime paths")
		if jsonOutput {
			return printConfigPathsJSON(c.out, cfg.DataDir, storage.DatabasePath(cfg.DataDir))
		}
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
	fmt.Fprint(c.out, formatCommandHelp(tokCommandSpec()))
}

func (c *CLI) printCommandHelp(path []string) error {
	spec, ok := findCommandSpec(path)
	if !ok {
		topic := strings.Join(path, " ")
		if topic == "" {
			topic = commandName
		}
		return &UsageError{
			Message: fmt.Sprintf("unknown help topic %q\n\nRun '%s help' for usage.", topic, commandName),
			Code:    2,
		}
	}
	fmt.Fprint(c.out, formatCommandHelp(spec))
	return nil
}

func (c *CLI) runVersion(args []string) error {
	jsonOutput, err := parseNoArgJSONOption(args, "version")
	if err != nil {
		return err
	}
	if jsonOutput {
		return printVersionJSON(c.out, c.version)
	}
	fmt.Fprintf(c.out, "%s %s\ncommit: %s\nbuilt: %s\n", commandName, c.version.Version, c.version.Commit, c.version.Date)
	return nil
}

func parseNoArgJSONOption(args []string, command string) (bool, error) {
	var jsonOutput bool
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		default:
			return false, &UsageError{Message: fmt.Sprintf("unknown %s option %q", command, arg), Code: 2}
		}
	}
	return jsonOutput, nil
}

func printVersionJSON(out io.Writer, version VersionInfo) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Commit  string `json:"commit"`
		BuiltAt string `json:"built_at"`
	}{
		Name:    commandName,
		Version: version.Version,
		Commit:  version.Commit,
		BuiltAt: version.Date,
	})
}

func printInitJSON(out io.Writer, dataDir, databasePath string) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(struct {
		DataDir      string `json:"data_dir"`
		DatabasePath string `json:"database_path"`
	}{
		DataDir:      dataDir,
		DatabasePath: databasePath,
	})
}

func printConfigPathsJSON(out io.Writer, dataDir, databasePath string) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(struct {
		DataDir      string `json:"data_dir"`
		DatabasePath string `json:"database_path"`
	}{
		DataDir:      dataDir,
		DatabasePath: databasePath,
	})
}
