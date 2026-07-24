package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		"name | display_name | path",
		"tok",
		"TOK",
		projectDir,
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

	var addJSONOut bytes.Buffer
	addJSONCLI := newProjectTestCLI(t.TempDir(), &addJSONOut)
	jsonProjectDir := t.TempDir()
	if err := addJSONCLI.Run(ctx, []string{"project", "add", jsonProjectDir, "--name", "json-tok", "--display-name", "JSON TOK", "--json"}); err != nil {
		t.Fatalf("project add --json returned error: %v", err)
	}
	var added projectOutput
	if err := json.Unmarshal(addJSONOut.Bytes(), &added); err != nil {
		t.Fatalf("parse project add JSON: %v\n%s", err, addJSONOut.String())
	}
	if added.ID == 0 || added.Name != "json-tok" || added.DisplayName != "JSON TOK" || added.Path != jsonProjectDir || added.CreatedAt == "" || added.UpdatedAt == "" {
		t.Fatalf("unexpected project add JSON: %+v", added)
	}

	var listJSONOut bytes.Buffer
	listJSONCLI := newProjectTestCLI(dataDir, &listJSONOut)
	if err := listJSONCLI.Run(ctx, []string{"project", "list", "--json"}); err != nil {
		t.Fatalf("project list --json returned error: %v", err)
	}
	var listed []projectOutput
	if err := json.Unmarshal(listJSONOut.Bytes(), &listed); err != nil {
		t.Fatalf("parse project list JSON: %v\n%s", err, listJSONOut.String())
	}
	if len(listed) != 1 || listed[0].Name != "tok" || listed[0].DisplayName != "TOK" || listed[0].Path != projectDir {
		t.Fatalf("unexpected project list JSON: %+v", listed)
	}

	var showJSONOut bytes.Buffer
	showJSONCLI := newProjectTestCLI(dataDir, &showJSONOut)
	if err := showJSONCLI.Run(ctx, []string{"project", "show", "tok", "--json"}); err != nil {
		t.Fatalf("project show --json returned error: %v", err)
	}
	var shown projectOutput
	if err := json.Unmarshal(showJSONOut.Bytes(), &shown); err != nil {
		t.Fatalf("parse project show JSON: %v\n%s", err, showJSONOut.String())
	}
	if shown.ID == 0 || shown.Name != "tok" || shown.DisplayName != "TOK" || shown.Path != projectDir || shown.CreatedAt == "" || shown.UpdatedAt == "" {
		t.Fatalf("unexpected project show JSON: %+v", shown)
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

func TestCLIProjectInstructionLifecycle(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}

	var addOut bytes.Buffer
	addCLI := newProjectTestCLI(dataDir, &addOut)
	if err := addCLI.Run(ctx, []string{
		"project", "instruction", "add",
		"--project", "tok",
		"--title", "Use Context7",
		"--body", "Use Context7 for library documentation.",
		"--priority", "high",
		"--json",
	}); err != nil {
		t.Fatalf("project instruction add returned error: %v", err)
	}
	var added projectInstructionOutput
	if err := json.Unmarshal(addOut.Bytes(), &added); err != nil {
		t.Fatalf("parse instruction add JSON: %v\n%s", err, addOut.String())
	}
	if added.ID == 0 || added.Scope != "project" || added.Title != "Use Context7" || added.Priority != "high" || !added.Enabled {
		t.Fatalf("unexpected added instruction: %+v", added)
	}

	var listOut bytes.Buffer
	listCLI := newProjectTestCLI(dataDir, &listOut)
	if err := listCLI.Run(ctx, []string{"project", "instruction", "list", "--project", "tok"}); err != nil {
		t.Fatalf("project instruction list returned error: %v", err)
	}
	for _, want := range []string{"id | priority | enabled | title", "high", "true", "Use Context7"} {
		if !strings.Contains(listOut.String(), want) {
			t.Fatalf("instruction list output missing %q:\n%s", want, listOut.String())
		}
	}

	disableCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := disableCLI.Run(ctx, []string{"project", "instruction", "disable", "--project", "tok", fmt.Sprintf("%d", added.ID)}); err != nil {
		t.Fatalf("project instruction disable returned error: %v", err)
	}

	var enabledListOut bytes.Buffer
	enabledListCLI := newProjectTestCLI(dataDir, &enabledListOut)
	if err := enabledListCLI.Run(ctx, []string{"project", "instruction", "list", "--project", "tok", "--json"}); err != nil {
		t.Fatalf("project instruction enabled list returned error: %v", err)
	}
	var enabled []projectInstructionOutput
	if err := json.Unmarshal(enabledListOut.Bytes(), &enabled); err != nil {
		t.Fatalf("parse enabled instruction list JSON: %v\n%s", err, enabledListOut.String())
	}
	if len(enabled) != 0 {
		t.Fatalf("disabled instruction appeared in enabled list: %+v", enabled)
	}

	var disabledListOut bytes.Buffer
	disabledListCLI := newProjectTestCLI(dataDir, &disabledListOut)
	if err := disabledListCLI.Run(ctx, []string{"project", "instruction", "list", "--project", "tok", "--include-disabled", "--json"}); err != nil {
		t.Fatalf("project instruction disabled list returned error: %v", err)
	}
	var disabled []projectInstructionOutput
	if err := json.Unmarshal(disabledListOut.Bytes(), &disabled); err != nil {
		t.Fatalf("parse disabled instruction list JSON: %v\n%s", err, disabledListOut.String())
	}
	if len(disabled) != 1 || disabled[0].Enabled {
		t.Fatalf("expected disabled instruction in include-disabled list: %+v", disabled)
	}

	removeCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := removeCLI.Run(ctx, []string{"project", "instruction", "remove", "--project", "tok", fmt.Sprintf("%d", added.ID)}); err != nil {
		t.Fatalf("project instruction remove returned error: %v", err)
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
