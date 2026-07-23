package app

import (
	"context"
	"fmt"
	"io"
	"strings"
)

const commandName = "tok"

// CLI owns the root command surface shared by process entrypoints and tests.
type CLI struct {
	out     io.Writer
	err     io.Writer
	version VersionInfo
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
		out:     out,
		err:     err,
		version: version,
	}
}

func (c *CLI) Run(ctx context.Context, args []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(args) == 0 {
		c.printHelp()
		return nil
	}

	switch args[0] {
	case "help", "-h", "--help":
		c.printHelp()
		return nil
	case "version", "-v", "--version":
		c.printVersion()
		return nil
	default:
		return &UsageError{
			Message: fmt.Sprintf("unknown command %q\n\nRun '%s help' for usage.", args[0], commandName),
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
  tok <command>

Commands:
  version   Print build version information
  help      Show this help
`
