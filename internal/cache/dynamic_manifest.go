package cache

import (
	"bufio"
	"fmt"
	"helm/internal/artifactresolve"
	"helm/internal/ir"
	"os"
	"path/filepath"
	"strings"
)

// DynamicManifestBootstrapMiss reports whether cache must run because a configured
// dynamic manifest path does not exist yet (clean clone / first build).
func DynamicManifestBootstrapMiss(
	helmBaseDir string,
	artifacts *ir.HelmArtifacts,
	globalVars map[string]string,
	parameters map[string]string,
) (bool, error) {
	if artifacts == nil || len(artifacts.Dynamic) == 0 {
		return false, nil
	}

	manifestPaths, err := artifactresolve.ArtifactResolveItems(
		helmBaseDir,
		artifacts.Dynamic,
		globalVars,
		parameters,
		false,
	)
	if err != nil {
		return false, err
	}

	for _, manifestPath := range manifestPaths {
		if _, err := os.Stat(manifestPath); err != nil {
			if os.IsNotExist(err) {
				return true, nil
			}
			return false, fmt.Errorf("dynamic manifest '%s': %w", manifestPath, err)
		}
	}
	return false, nil
}

// DynamicManifestDiscoveredPaths reads each configured manifest (one discovered path
// per non-empty line) and returns absolute paths whose file contents should be hashed
// for cache state. Manifest files themselves are not included.
func DynamicManifestDiscoveredPaths(
	helmBaseDir string,
	artifacts *ir.HelmArtifacts,
	globalVars map[string]string,
	parameters map[string]string,
) ([]string, error) {
	if artifacts == nil || len(artifacts.Dynamic) == 0 {
		return nil, nil
	}

	manifestPaths, err := artifactresolve.ArtifactResolveItems(
		helmBaseDir,
		artifacts.Dynamic,
		globalVars,
		parameters,
		false,
	)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var discovered []string

	for _, manifestPath := range manifestPaths {
		lines, err := cacheReadManifestLines(manifestPath)
		if err != nil {
			return nil, err
		}
		for _, line := range lines {
			absolute, err := cacheAnchorDiscoveredPath(helmBaseDir, line)
			if err != nil {
				return nil, fmt.Errorf("dynamic manifest '%s': %w", manifestPath, err)
			}
			if _, exists := seen[absolute]; exists {
				continue
			}
			seen[absolute] = struct{}{}
			discovered = append(discovered, absolute)
		}
	}

	return discovered, nil
}

func cacheReadManifestLines(manifestPath string) ([]string, error) {
	file, err := os.Open(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest '%s': %w", manifestPath, err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read manifest '%s': %w", manifestPath, err)
	}
	return lines, nil
}

func cacheAnchorDiscoveredPath(helmBaseDir, path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty path in manifest")
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	if helmBaseDir == "" {
		return filepath.Clean(path), nil
	}
	return filepath.Clean(filepath.Join(helmBaseDir, path)), nil
}
