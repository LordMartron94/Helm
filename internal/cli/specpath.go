package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

func ResolveHelmLSpecPath() (string, error) {
	if path := os.Getenv("HELM_LSPEC_PATH"); path != "" {
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("HELM_LSPEC_PATH not usable (%q): %w", path, err)
		}
		return path, nil
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	path, err := findProjectFileUpwards(dir, "libs/lingua/helm/helm.lspec")
	if err != nil {
		return "", fmt.Errorf("%w; set HELM_LSPEC_PATH", err)
	}

	return path, nil
}

func findProjectFileUpwards(startDir, relativePath string) (string, error) {
	dir := startDir

	for {
		candidate := filepath.Join(dir, relativePath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("could not find %s from cwd", relativePath)
}
