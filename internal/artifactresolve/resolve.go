package artifactresolve

import (
	"fmt"
	"helm/internal/expand"
	"helm/internal/ir"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func ArtifactResolveInputPaths(
	helmBaseDir string,
	artifacts *ir.HelmArtifacts,
	globalVars map[string]string,
	parameters map[string]string,
) ([]string, error) {
	if artifacts == nil {
		return nil, nil
	}
	return artifactResolvePaths(helmBaseDir, artifacts.Inputs, globalVars, parameters, true)
}

func ArtifactResolveOutputPaths(
	helmBaseDir string,
	outputs []ir.HelmArtifactInput,
	globalVars map[string]string,
	parameters map[string]string,
) ([]string, error) {
	return artifactResolvePaths(helmBaseDir, outputs, globalVars, parameters, false)
}

func artifactResolvePaths(
	helmBaseDir string,
	items []ir.HelmArtifactInput,
	globalVars map[string]string,
	parameters map[string]string,
	requireExistingFiles bool,
) ([]string, error) {
	pathSet := map[string]struct{}{}
	for _, item := range items {
		paths, err := artifactResolveItem(helmBaseDir, item, globalVars, parameters, requireExistingFiles)
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			pathSet[path] = struct{}{}
		}
	}
	return artifactSortedPaths(pathSet), nil
}

func artifactResolveItem(
	helmBaseDir string,
	item ir.HelmArtifactInput,
	globalVars map[string]string,
	parameters map[string]string,
	requireExistingFiles bool,
) ([]string, error) {
	switch item.Kind {
	case ir.ArtifactInputString:
		path := expand.ExpandInterpolateLiteral(item.Literal, globalVars, parameters)
		if path == "" {
			return nil, nil
		}
		path = artifactAnchorPath(helmBaseDir, path)
		if requireExistingFiles {
			if err := artifactEnsureFile(path); err != nil {
				return nil, err
			}
		}
		return []string{path}, nil
	case ir.ArtifactInputGlob:
		if item.Glob == nil {
			return nil, fmt.Errorf("glob artifact item is missing glob configuration")
		}
		glob := ArtifactGlobWithInterpolatedBase(item.Glob, globalVars, parameters)
		return ArtifactWalkGlob(helmBaseDir, glob)
	default:
		return nil, fmt.Errorf("unknown artifact item kind")
	}
}

func ArtifactGlobWithInterpolatedBase(
	glob *ir.HelmGlob,
	globalVars map[string]string,
	parameters map[string]string,
) *ir.HelmGlob {
	if glob == nil {
		return nil
	}
	copy := *glob
	copy.BaseDirectory = expand.ExpandInterpolateLiteral(glob.BaseDirectory, globalVars, parameters)
	copy.Includes = expandInterpolateGlobPatterns(glob.Includes, globalVars, parameters)
	copy.Excludes = expandInterpolateGlobPatterns(glob.Excludes, globalVars, parameters)
	copy.Types = expand.ExpandInterpolateLiteral(glob.Types, globalVars, parameters)
	return &copy
}

func expandInterpolateGlobPatterns(
	patterns []string,
	globalVars map[string]string,
	parameters map[string]string,
) []string {
	if len(patterns) == 0 {
		return nil
	}
	out := make([]string, len(patterns))
	for i, pattern := range patterns {
		out[i] = expand.ExpandInterpolateLiteral(pattern, globalVars, parameters)
	}
	return out
}

func artifactAnchorPath(helmBaseDir, path string) string {
	if path == "" || helmBaseDir == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(helmBaseDir, path)
}

func artifactEnsureFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("artifact path '%s': %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("artifact path '%s' is a directory, expected a file", path)
	}
	return nil
}

func artifactSortedPaths(pathSet map[string]struct{}) []string {
	out := make([]string, 0, len(pathSet))
	for path := range pathSet {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func artifactPathMatchesExclude(relPath string, exclude string) bool {
	if exclude == "" {
		return false
	}
	matched, err := filepath.Match(exclude, relPath)
	if err != nil {
		return strings.Contains(relPath, exclude)
	}
	return matched
}
