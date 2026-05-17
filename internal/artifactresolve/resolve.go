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

	pathSet := map[string]struct{}{}
	for _, input := range artifacts.Inputs {
		paths, err := artifactResolveInputItem(helmBaseDir, input, globalVars, parameters)
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			pathSet[path] = struct{}{}
		}
	}

	return artifactSortedPaths(pathSet), nil
}

func ArtifactResolveOutputPaths(
	helmBaseDir string,
	outputs []string,
	globalVars map[string]string,
	parameters map[string]string,
) ([]string, error) {
	pathSet := map[string]struct{}{}
	for _, literal := range outputs {
		resolved := expand.ExpandInterpolateLiteral(literal, globalVars, parameters)
		if resolved == "" {
			continue
		}
		pathSet[artifactAnchorPath(helmBaseDir, resolved)] = struct{}{}
	}

	return artifactSortedPaths(pathSet), nil
}

func artifactResolveInputItem(
	helmBaseDir string,
	input ir.HelmArtifactInput,
	globalVars map[string]string,
	parameters map[string]string,
) ([]string, error) {
	switch input.Kind {
	case ir.ArtifactInputString:
		path := expand.ExpandInterpolateLiteral(input.Literal, globalVars, parameters)
		if path == "" {
			return nil, nil
		}
		path = artifactAnchorPath(helmBaseDir, path)
		if err := artifactEnsureFile(path); err != nil {
			return nil, err
		}
		return []string{path}, nil
	case ir.ArtifactInputGlob:
		if input.Glob == nil {
			return nil, fmt.Errorf("glob artifact input is missing glob configuration")
		}
		return ArtifactWalkGlob(helmBaseDir, input.Glob)
	default:
		return nil, fmt.Errorf("unknown artifact input kind")
	}
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
