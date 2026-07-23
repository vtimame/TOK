package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

const (
	AppDirName    = "tok"
	EnvConfigFile = "TOK_CONFIG"
	EnvDataDir    = "TOK_DATA_DIR"
)

type Config struct {
	DataDir string    `yaml:"data_dir"`
	Log     LogConfig `yaml:"log"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type LoadOptions struct {
	ConfigPath string
	Env        map[string]string
	HomeDir    string
	GOOS       string
}

func Load(opts LoadOptions) (Config, error) {
	cfg := Default(opts)

	configPath := opts.ConfigPath
	if configPath == "" {
		configPath = opts.Env[EnvConfigFile]
	}

	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return Config{}, fmt.Errorf("read config file %q: %w", configPath, err)
		}

		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(&cfg); err != nil {
			return Config{}, fmt.Errorf("parse config file %q: %w", configPath, err)
		}
	}

	if dataDir := opts.Env[EnvDataDir]; dataDir != "" {
		cfg.DataDir = dataDir
	}

	if cfg.DataDir == "" {
		return Config{}, errors.New("resolve data directory: home directory is empty")
	}

	if !filepath.IsAbs(cfg.DataDir) {
		return Config{}, fmt.Errorf("data directory must be absolute: %s", cfg.DataDir)
	}

	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = "json"
	}

	return cfg, nil
}

func Default(opts LoadOptions) Config {
	return Config{
		DataDir: defaultDataDir(opts),
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

func LoadFromOS(configPath string) (Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("resolve home directory: %w", err)
	}

	return Load(LoadOptions{
		ConfigPath: configPath,
		Env: map[string]string{
			EnvConfigFile:   os.Getenv(EnvConfigFile),
			EnvDataDir:      os.Getenv(EnvDataDir),
			"LOCALAPPDATA":  os.Getenv("LOCALAPPDATA"),
			"XDG_DATA_HOME": os.Getenv("XDG_DATA_HOME"),
		},
		HomeDir: homeDir,
		GOOS:    runtime.GOOS,
	})
}

func defaultDataDir(opts LoadOptions) string {
	if opts.Env[EnvDataDir] != "" {
		return opts.Env[EnvDataDir]
	}

	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}

	home := opts.HomeDir
	if home == "" {
		return ""
	}

	switch goos {
	case "windows":
		if appData := opts.Env["LOCALAPPDATA"]; appData != "" {
			return filepath.Join(appData, AppDirName)
		}
		return filepath.Join(home, "AppData", "Local", AppDirName)
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", AppDirName)
	default:
		if xdgDataHome := opts.Env["XDG_DATA_HOME"]; xdgDataHome != "" {
			return filepath.Join(xdgDataHome, AppDirName)
		}
		return filepath.Join(home, ".local", "share", AppDirName)
	}
}
