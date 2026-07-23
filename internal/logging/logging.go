package logging

import (
	"fmt"
	"io"
	"time"

	"github.com/rs/zerolog"

	"s26.sh/tok/internal/config"
)

func NewLogger(out io.Writer, cfg config.LogConfig) (zerolog.Logger, error) {
	level := cfg.Level
	if level == "" {
		level = "info"
	}

	parsedLevel, err := zerolog.ParseLevel(level)
	if err != nil {
		return zerolog.Logger{}, fmt.Errorf("parse log level %q: %w", level, err)
	}

	format := cfg.Format
	if format == "" {
		format = "json"
	}

	var writer io.Writer = out
	switch format {
	case "json":
	case "console":
		writer = zerolog.ConsoleWriter{
			Out:        out,
			TimeFormat: time.RFC3339,
		}
	default:
		return zerolog.Logger{}, fmt.Errorf("unknown log format %q", format)
	}

	return zerolog.New(writer).Level(parsedLevel).With().Timestamp().Logger(), nil
}
