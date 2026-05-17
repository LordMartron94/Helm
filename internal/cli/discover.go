package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const helmfileDefaultName = "Helmfile"

func DiscoverHelmFile(explicitPath string) (string, error) {
	if explicitPath != "" {
		info, err := os.Stat(explicitPath)
		if err != nil {
			return "", fmt.Errorf("helm file %q: %w", explicitPath, err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("helm file %q is a directory", explicitPath)
		}
		abs, err := filepath.Abs(explicitPath)
		if err != nil {
			return "", err
		}
		return abs, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	helmfilePath := filepath.Join(cwd, helmfileDefaultName)
	if info, err := os.Stat(helmfilePath); err == nil && !info.IsDir() {
		return helmfilePath, nil
	}

	helmFiles, err := filepath.Glob(filepath.Join(cwd, "*.helm"))
	if err != nil {
		return "", err
	}

	switch len(helmFiles) {
	case 0:
		return "", fmt.Errorf(
			"no helm file found in %q: expected %q or exactly one *.helm file",
			cwd,
			helmfileDefaultName,
		)
	case 1:
		abs, err := filepath.Abs(helmFiles[0])
		if err != nil {
			return "", err
		}
		return abs, nil
	default:
		var names []string
		for _, path := range helmFiles {
			names = append(names, filepath.Base(path))
		}
		return "", fmt.Errorf(
			"multiple *.helm files in %q, pass the helm file explicitly: %s",
			cwd,
			strings.Join(names, ", "),
		)
	}
}
