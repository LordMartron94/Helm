package cache

import (
	"bufio"
	"fmt"
	"helm/internal/artifactresolve"
	"helm/internal/expand"
	"helm/internal/ir"
	"helm/internal/workspacepath"
	"os"
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
	scalars := expand.InterpolationContextMergeScalars(nil, globalVars)
	scalars = expand.InterpolationContextMergeScalars(scalars, parameters)
	return DynamicManifestBootstrapMissContext(
		helmBaseDir,
		artifacts,
		expand.InterpolationContext{Scalars: scalars},
	)
}

func DynamicManifestBootstrapMissContext(
	helmBaseDir string,
	artifacts *ir.HelmArtifacts,
	ctx expand.InterpolationContext,
) (bool, error) {
	if artifacts == nil || len(artifacts.Dynamic) == 0 {
		return false, nil
	}

	manifestPaths, err := artifactresolve.ArtifactResolveItemsContext(
		helmBaseDir,
		artifacts.Dynamic,
		ctx,
		false,
	)
	if err != nil {
		return false, err
	}

	for _, manifestPath := range manifestPaths {
		absPath := workspacepath.WorkspaceAnchor(helmBaseDir, manifestPath)
		if _, err := os.Stat(absPath); err != nil {
			if os.IsNotExist(err) {
				return true, nil
			}
			return false, fmt.Errorf("dynamic manifest '%s': %w", manifestPath, err)
		}
	}
	return false, nil
}

// DynamicManifestDiscoveredPaths reads each configured manifest (one discovered path
// per non-empty line) and returns workspace-relative paths whose file contents should
// be hashed for cache state. Manifest files themselves are not included. Paths outside
// the workspace are silently dropped.
func DynamicManifestDiscoveredPaths(
	helmBaseDir string,
	artifacts *ir.HelmArtifacts,
	globalVars map[string]string,
	parameters map[string]string,
) ([]string, error) {
	scalars := expand.InterpolationContextMergeScalars(nil, globalVars)
	scalars = expand.InterpolationContextMergeScalars(scalars, parameters)
	return DynamicManifestDiscoveredPathsContext(
		helmBaseDir,
		artifacts,
		expand.InterpolationContext{Scalars: scalars},
	)
}

func DynamicManifestDiscoveredPathsContext(
	helmBaseDir string,
	artifacts *ir.HelmArtifacts,
	ctx expand.InterpolationContext,
) ([]string, error) {
	if artifacts == nil || len(artifacts.Dynamic) == 0 {
		return nil, nil
	}

	manifestPaths, err := artifactresolve.ArtifactResolveItemsContext(
		helmBaseDir,
		artifacts.Dynamic,
		ctx,
		false,
	)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var discovered []string

	for _, manifestPath := range manifestPaths {
		absManifest := workspacepath.WorkspaceAnchor(helmBaseDir, manifestPath)
		lines, err := cacheReadManifestLines(absManifest)
		if err != nil {
			return nil, err
		}
		for _, line := range lines {
			rel, ok := workspacepath.WorkspaceRelative(helmBaseDir, line)
			if !ok {
				continue
			}
			if _, exists := seen[rel]; exists {
				continue
			}
			seen[rel] = struct{}{}
			discovered = append(discovered, rel)
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
