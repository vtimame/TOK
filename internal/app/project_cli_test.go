package app

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"s26.sh/tok/internal/config"
)

func TestCLIProjectAddListShow(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	var addOut bytes.Buffer
	addCLI := newProjectTestCLI(dataDir, &addOut)
	if err := addCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok", "--display-name", "TOK"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}

	addOutput := addOut.String()
	for _, want := range []string{
		"name: tok",
		"display_name: TOK",
		"path: " + projectDir,
		"created_at:",
		"updated_at:",
	} {
		if !strings.Contains(addOutput, want) {
			t.Fatalf("project add output missing %q:\n%s", want, addOutput)
		}
	}

	var listOut bytes.Buffer
	listCLI := newProjectTestCLI(dataDir, &listOut)
	if err := listCLI.Run(ctx, []string{"project", "list"}); err != nil {
		t.Fatalf("project list returned error: %v", err)
	}

	listOutput := listOut.String()
	for _, want := range []string{
		"name\tdisplay_name\tpath",
		"tok\tTOK\t" + projectDir,
	} {
		if !strings.Contains(listOutput, want) {
			t.Fatalf("project list output missing %q:\n%s", want, listOutput)
		}
	}

	var showOut bytes.Buffer
	showCLI := newProjectTestCLI(dataDir, &showOut)
	if err := showCLI.Run(ctx, []string{"project", "show", "tok"}); err != nil {
		t.Fatalf("project show returned error: %v", err)
	}

	showOutput := showOut.String()
	for _, want := range []string{
		"name: tok",
		"display_name: TOK",
		"path: " + projectDir,
		"created_at:",
		"updated_at:",
	} {
		if !strings.Contains(showOutput, want) {
			t.Fatalf("project show output missing %q:\n%s", want, showOutput)
		}
	}
}

func TestCLIProjectAddDefaultsDisplayNameToName(t *testing.T) {
	var out bytes.Buffer
	cli := newProjectTestCLI(t.TempDir(), &out)

	if err := cli.Run(context.Background(), []string{"project", "add", t.TempDir(), "--name=tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}

	if !strings.Contains(out.String(), "display_name: tok") {
		t.Fatalf("expected display_name to default to name:\n%s", out.String())
	}
}

func TestCLIProjectAddRejectsMissingName(t *testing.T) {
	cli := newProjectTestCLI(t.TempDir(), &bytes.Buffer{})

	err := cli.Run(context.Background(), []string{"project", "add", t.TempDir()})
	if err == nil {
		t.Fatal("expected error")
	}

	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected UsageError, got %T: %v", err, err)
	}
	if usageErr.Code != 2 || !strings.Contains(usageErr.Message, "requires --name") {
		t.Fatalf("unexpected usage error: %+v", usageErr)
	}
}

func TestCLIProjectAddRejectsMissingPath(t *testing.T) {
	cli := newProjectTestCLI(t.TempDir(), &bytes.Buffer{})
	missingPath := filepath.Join(t.TempDir(), "missing")

	err := cli.Run(context.Background(), []string{"project", "add", missingPath, "--name", "missing"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "project path does not exist") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newProjectTestCLI(dataDir string, out *bytes.Buffer) *CLI {
	cli := NewCLI(out, &bytes.Buffer{}, VersionInfo{})
	cli.loadCfg = func(string) (config.Config, error) {
		return config.Config{
			DataDir: dataDir,
			Log: config.LogConfig{
				Level:  "info",
				Format: "json",
			},
		}, nil
	}
	return cli
}
