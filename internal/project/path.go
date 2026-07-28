package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ValidateLocalPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("project path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve project path %q: %w", path, err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("project path does not exist: %s", absPath)
		}
		return "", fmt.Errorf("inspect project path %q: %w", absPath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project path must be a directory: %s", absPath)
	}

	return absPath, nil
}
