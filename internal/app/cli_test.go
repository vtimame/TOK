package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"s26.sh/tok/internal/config"
	"s26.sh/tok/internal/storage"
)

func TestCLIRootPrintsHelp(t *testing.T) {
	var out bytes.Buffer
	cli := NewCLI(&out, &bytes.Buffer{}, VersionInfo{})

	if err := cli.Run(context.Background(), nil); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{"TOK - Task Operations Kernel", "Usage:", "version"} {
		if !strings.Contains(got, want) {
			t.Fatalf("help output missing %q:\n%s", want, got)
		}
	}
}

func TestCLINestedHelpPrintsCommandAndSubcommandUsage(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "project flag help",
			args: []string{"project", "--help"},
			want: []string{"project - Register and inspect local projects", "Usage:", "tok project <command>", "add", "list", "show"},
		},
		{
			name: "help command path",
			args: []string{"help", "task", "dependency"},
			want: []string{"dependency - Manage task dependencies", "tok task dependency <command>", "add", "remove"},
		},
		{
			name: "deep index ignore add help",
			args: []string{"index", "ignore", "add", "--help"},
			want: []string{"add - Add one ignore pattern", "tok index ignore add --project <name> [--json] <pattern>", "--project <name>", "--json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			cli := NewCLI(&out, &bytes.Buffer{}, VersionInfo{})

			if err := cli.Run(context.Background(), tt.args); err != nil {
				t.Fatalf("Run returned error: %v", err)
			}

			got := out.String()
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Fatalf("help output missing %q:\n%s", want, got)
				}
			}
		})
	}
}

func TestCLIUnknownHelpTopicReturnsUsageError(t *testing.T) {
	cli := NewCLI(&bytes.Buffer{}, &bytes.Buffer{}, VersionInfo{})

	err := cli.Run(context.Background(), []string{"help", "missing"})
	if err == nil {
		t.Fatal("expected error")
	}

	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected UsageError, got %T: %v", err, err)
	}
	if usageErr.Code != 2 || !strings.Contains(usageErr.Message, "unknown help topic") {
		t.Fatalf("unexpected usage error: %+v", usageErr)
	}
}

func TestCLICompletionScripts(t *testing.T) {
	tests := []struct {
		shell string
		want  []string
	}{
		{shell: "bash", want: []string{"_tok_completion", "complete -F _tok_completion tok", "tok __complete"}},
		{shell: "zsh", want: []string{"#compdef tok", "compdef _tok tok", "tok __complete"}},
		{shell: "fish", want: []string{"function __tok_complete", "complete -c tok", "tok __complete"}},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			var out bytes.Buffer
			cli := NewCLI(&out, &bytes.Buffer{}, VersionInfo{})

			if err := cli.Run(context.Background(), []string{"completion", tt.shell}); err != nil {
				t.Fatalf("Run returned error: %v", err)
			}

			got := out.String()
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Fatalf("completion script missing %q:\n%s", want, got)
				}
			}
		})
	}
}

func TestCLICompleteSuggestsCommandsFlagsAndProjectNames(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	addCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := addCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "root commands", args: []string{"__complete", ""}, want: []string{"project", "completion", "index"}},
		{name: "nested commands", args: []string{"__complete", "index", "ignore", ""}, want: []string{"add", "list", "refresh", "remove"}},
		{name: "flags", args: []string{"__complete", "index", "update", "--"}, want: []string{"--all", "--json", "--project"}},
		{name: "project names", args: []string{"__complete", "index", "update", "--project", ""}, want: []string{"tok"}},
		{name: "completion shells", args: []string{"__complete", "completion", ""}, want: []string{"bash", "fish", "zsh"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			cli := newProjectTestCLI(dataDir, &out)

			if err := cli.Run(ctx, tt.args); err != nil {
				t.Fatalf("Run returned error: %v", err)
			}

			got := out.String()
			for _, want := range tt.want {
				if !strings.Contains(got, want+"\n") {
					t.Fatalf("completion output missing %q:\n%s", want, got)
				}
			}
		})
	}
}

func TestCLIVersionPrintsBuildInfo(t *testing.T) {
	var out bytes.Buffer
	cli := NewCLI(&out, &bytes.Buffer{}, VersionInfo{
		Version: "test",
		Commit:  "abc123",
		Date:    "2026-07-23T00:00:00Z",
	})

	if err := cli.Run(context.Background(), []string{"version"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{"tok test", "commit: abc123", "built: 2026-07-23T00:00:00Z"} {
		if !strings.Contains(got, want) {
			t.Fatalf("version output missing %q:\n%s", want, got)
		}
	}
}

func TestCLIVersionPrintsJSON(t *testing.T) {
	var out bytes.Buffer
	cli := NewCLI(&out, &bytes.Buffer{}, VersionInfo{
		Version: "test",
		Commit:  "abc123",
		Date:    "2026-07-23T00:00:00Z",
	})

	if err := cli.Run(context.Background(), []string{"version", "--json"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	var got struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Commit  string `json:"commit"`
		BuiltAt string `json:"built_at"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("parse version JSON: %v\n%s", err, out.String())
	}
	if got.Name != "tok" || got.Version != "test" || got.Commit != "abc123" || got.BuiltAt != "2026-07-23T00:00:00Z" {
		t.Fatalf("unexpected version JSON: %+v", got)
	}
}

func TestCLIUnknownCommandReturnsUsageError(t *testing.T) {
	cli := NewCLI(&bytes.Buffer{}, &bytes.Buffer{}, VersionInfo{})

	err := cli.Run(context.Background(), []string{"missing"})
	if err == nil {
		t.Fatal("expected error")
	}

	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected UsageError, got %T: %v", err, err)
	}
	if usageErr.Code != 2 {
		t.Fatalf("expected code 2, got %d", usageErr.Code)
	}
}

func TestCLIConfigPathsPrintsResolvedDataDir(t *testing.T) {
	var out bytes.Buffer
	cli := NewCLI(&out, &bytes.Buffer{}, VersionInfo{})
	cli.loadCfg = func(path string) (config.Config, error) {
		if path != "/tmp/tok.yaml" {
			t.Fatalf("unexpected config path: %s", path)
		}
		return config.Config{
			DataDir: "/tmp/tok-data",
			Log: config.LogConfig{
				Level:  "info",
				Format: "json",
			},
		}, nil
	}

	if err := cli.Run(context.Background(), []string{"--config", "/tmp/tok.yaml", "config", "paths"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "data_dir: /tmp/tok-data") {
		t.Fatalf("unexpected config paths output:\n%s", got)
	}
}

func TestCLIConfigPathsPrintsJSON(t *testing.T) {
	var out bytes.Buffer
	cli := NewCLI(&out, &bytes.Buffer{}, VersionInfo{})
	cli.loadCfg = func(path string) (config.Config, error) {
		if path != "/tmp/tok.yaml" {
			t.Fatalf("unexpected config path: %s", path)
		}
		return config.Config{
			DataDir: "/tmp/tok-data",
			Log: config.LogConfig{
				Level:  "info",
				Format: "json",
			},
		}, nil
	}

	if err := cli.Run(context.Background(), []string{"--config", "/tmp/tok.yaml", "config", "paths", "--json"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	var got struct {
		DataDir      string `json:"data_dir"`
		DatabasePath string `json:"database_path"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("parse config paths JSON: %v\n%s", err, out.String())
	}
	if got.DataDir != "/tmp/tok-data" || got.DatabasePath != storage.DatabasePath("/tmp/tok-data") {
		t.Fatalf("unexpected config paths JSON: %+v", got)
	}
}

func TestCLIConfigPathsAppliesLogLevelFlag(t *testing.T) {
	var errOut bytes.Buffer
	cli := NewCLI(&bytes.Buffer{}, &errOut, VersionInfo{})
	cli.loadCfg = func(string) (config.Config, error) {
		return config.Config{
			DataDir: "/tmp/tok-data",
			Log: config.LogConfig{
				Level:  "info",
				Format: "json",
			},
		}, nil
	}

	if err := cli.Run(context.Background(), []string{"--log-level", "debug", "config", "paths"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got := errOut.String()
	if !strings.Contains(got, `"level":"debug"`) {
		t.Fatalf("expected debug log output:\n%s", got)
	}
}

func TestCLIInitInitializesRuntimeDatabase(t *testing.T) {
	dataDir := t.TempDir()
	var out bytes.Buffer
	cli := NewCLI(&out, &bytes.Buffer{}, VersionInfo{})
	cli.loadCfg = func(string) (config.Config, error) {
		return config.Config{
			DataDir: dataDir,
			Log: config.LogConfig{
				Level:  "info",
				Format: "json",
			},
		}, nil
	}

	if err := cli.Run(context.Background(), []string{"init"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	dbPath := filepath.Join(dataDir, storage.DatabaseFileName)
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected initialized database at %s: %v", dbPath, err)
	}

	got := out.String()
	if !strings.Contains(got, "initialized database: "+dbPath) {
		t.Fatalf("unexpected init output:\n%s", got)
	}
}

func TestCLIInitPrintsJSON(t *testing.T) {
	dataDir := t.TempDir()
	var out bytes.Buffer
	cli := NewCLI(&out, &bytes.Buffer{}, VersionInfo{})
	cli.loadCfg = func(string) (config.Config, error) {
		return config.Config{
			DataDir: dataDir,
			Log: config.LogConfig{
				Level:  "info",
				Format: "json",
			},
		}, nil
	}

	if err := cli.Run(context.Background(), []string{"init", "--json"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	var got struct {
		DataDir      string `json:"data_dir"`
		DatabasePath string `json:"database_path"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("parse init JSON: %v\n%s", err, out.String())
	}
	if got.DataDir != dataDir || got.DatabasePath != storage.DatabasePath(dataDir) {
		t.Fatalf("unexpected init JSON: %+v", got)
	}
	if _, err := os.Stat(got.DatabasePath); err != nil {
		t.Fatalf("expected initialized database at %s: %v", got.DatabasePath, err)
	}
}
