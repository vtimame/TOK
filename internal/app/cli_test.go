package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
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
