package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsToXDGDataHome(t *testing.T) {
	cfg, err := Load(LoadOptions{
		Env: map[string]string{
			"XDG_DATA_HOME": "/tmp/xdg-data",
		},
		HomeDir: "/home/alice",
		GOOS:    "linux",
	})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.DataDir != "/tmp/xdg-data/tok" {
		t.Fatalf("unexpected data dir: %s", cfg.DataDir)
	}
	if cfg.Log.Level != "info" {
		t.Fatalf("unexpected log level: %s", cfg.Log.Level)
	}
	if cfg.Log.Format != "json" {
		t.Fatalf("unexpected log format: %s", cfg.Log.Format)
	}
}

func TestLoadUsesHomeFallback(t *testing.T) {
	cfg, err := Load(LoadOptions{
		Env:     map[string]string{},
		HomeDir: "/home/alice",
		GOOS:    "linux",
	})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.DataDir != "/home/alice/.local/share/tok" {
		t.Fatalf("unexpected data dir: %s", cfg.DataDir)
	}
}

func TestLoadUsesDarwinApplicationSupport(t *testing.T) {
	cfg, err := Load(LoadOptions{
		Env:     map[string]string{},
		HomeDir: "/Users/alice",
		GOOS:    "darwin",
	})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.DataDir != "/Users/alice/Library/Application Support/tok" {
		t.Fatalf("unexpected data dir: %s", cfg.DataDir)
	}
}

func TestLoadConfigFileOverridesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tok.yaml")
	if err := os.WriteFile(path, []byte("data_dir: /tmp/tok-data\nlog:\n  level: debug\n  format: console\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(LoadOptions{
		ConfigPath: path,
		Env:        map[string]string{},
		HomeDir:    "/home/alice",
		GOOS:       "linux",
	})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.DataDir != "/tmp/tok-data" {
		t.Fatalf("unexpected data dir: %s", cfg.DataDir)
	}
	if cfg.Log.Level != "debug" {
		t.Fatalf("unexpected log level: %s", cfg.Log.Level)
	}
	if cfg.Log.Format != "console" {
		t.Fatalf("unexpected log format: %s", cfg.Log.Format)
	}
}

func TestLoadEnvDataDirOverridesConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tok.yaml")
	if err := os.WriteFile(path, []byte("data_dir: /tmp/from-config\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(LoadOptions{
		ConfigPath: path,
		Env: map[string]string{
			EnvDataDir: "/tmp/from-env",
		},
		HomeDir: "/home/alice",
		GOOS:    "linux",
	})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.DataDir != "/tmp/from-env" {
		t.Fatalf("unexpected data dir: %s", cfg.DataDir)
	}
}

func TestLoadUsesConfigFileFromEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tok.yaml")
	if err := os.WriteFile(path, []byte("data_dir: /tmp/from-env-config\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(LoadOptions{
		Env: map[string]string{
			EnvConfigFile: path,
		},
		HomeDir: "/home/alice",
		GOOS:    "linux",
	})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.DataDir != "/tmp/from-env-config" {
		t.Fatalf("unexpected data dir: %s", cfg.DataDir)
	}
}

func TestLoadRejectsUnknownConfigFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tok.yaml")
	if err := os.WriteFile(path, []byte("unexpected: true\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(LoadOptions{
		ConfigPath: path,
		Env:        map[string]string{},
		HomeDir:    "/home/alice",
		GOOS:       "linux",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
