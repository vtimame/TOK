package logging

import (
	"bytes"
	"strings"
	"testing"

	"s26.sh/tok/internal/config"
)

func TestNewLoggerWritesJSON(t *testing.T) {
	var out bytes.Buffer
	logger, err := NewLogger(&out, config.LogConfig{
		Level:  "debug",
		Format: "json",
	})
	if err != nil {
		t.Fatalf("NewLogger returned error: %v", err)
	}

	logger.Debug().Str("component", "test").Msg("hello")

	got := out.String()
	for _, want := range []string{`"level":"debug"`, `"component":"test"`, `"message":"hello"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("log output missing %q:\n%s", want, got)
		}
	}
}

func TestNewLoggerHonorsLevel(t *testing.T) {
	var out bytes.Buffer
	logger, err := NewLogger(&out, config.LogConfig{
		Level:  "info",
		Format: "json",
	})
	if err != nil {
		t.Fatalf("NewLogger returned error: %v", err)
	}

	logger.Debug().Msg("hidden")
	logger.Info().Msg("visible")

	got := out.String()
	if strings.Contains(got, "hidden") {
		t.Fatalf("debug log should be filtered:\n%s", got)
	}
	if !strings.Contains(got, "visible") {
		t.Fatalf("info log should be written:\n%s", got)
	}
}

func TestNewLoggerRejectsInvalidLevel(t *testing.T) {
	_, err := NewLogger(&bytes.Buffer{}, config.LogConfig{
		Level:  "loud",
		Format: "json",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
