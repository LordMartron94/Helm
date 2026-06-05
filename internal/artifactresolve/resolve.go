package artifactresolve

import (
	"fmt"
	"helm/internal/expand"
	"helm/internal/ir"
	"os"
	"path/filepath"
	"sort"
)

func artifactInterpolationContext(
	globalVars map[string]string,
	parameters map[string]string,
) expand.InterpolationContext {
	scalars := expand.InterpolationContextMergeScalars(nil, globalVars)
	scalars = expand.InterpolationContextMergeScalars(scalars, parameters)
	return expand.InterpolationContext{Scalars: scalars}
}

func ArtifactResolveItems(
	helmBaseDir string,
	items []ir.HelmArtifactInput,
	globalVars map[string]string,
	parameters map[string]string,
	requireExistingFiles bool,
) ([]string, error) {
	return ArtifactResolveItemsContext(
		helmBaseDir,
		items,
		artifactInterpolationContext(globalVars, parameters),
		requireExistingFiles,
	)
}

func ArtifactResolveItemsContext(
	helmBaseDir string,
	items []ir.HelmArtifactInput,
	ctx expand.InterpolationContext,
	requireExistingFiles bool,
) ([]string, error) {
	return artifactResolvePathsContext(helmBaseDir, items, ctx, requireExistingFiles)
}

func ArtifactResolveInputPaths(
	helmBaseDir string,
	artifacts *ir.HelmArtifacts,
	globalVars map[string]string,
	parameters map[string]string,
) ([]string, error) {
	if artifacts == nil {
		return nil, nil
	}
	return artifactResolvePaths(
		helmBaseDir,
		artifacts.Inputs,
		globalVars,
		parameters,
		true,
	)
}

func ArtifactResolveInputPathsContext(
	helmBaseDir string,
	artifacts *ir.HelmArtifacts,
	ctx expand.InterpolationContext,
) ([]string, error) {
	if artifacts == nil {
		return nil, nil
	}
	return artifactResolvePathsContext(helmBaseDir, artifacts.Inputs, ctx, true)
}

func ArtifactResolveOutputPaths(
	helmBaseDir string,
	outputs []ir.HelmArtifactInput,
	globalVars map[string]string,
	parameters map[string]string,
) ([]string, error) {
	return artifactResolvePaths(helmBaseDir, outputs, globalVars, parameters, false)
}

func ArtifactResolveOutputPathsContext(
	helmBaseDir string,
	outputs []ir.HelmArtifactInput,
	ctx expand.InterpolationContext,
) ([]string, error) {
	return artifactResolvePathsContext(helmBaseDir, outputs, ctx, false)
}

func artifactResolvePaths(
	helmBaseDir string,
	items []ir.HelmArtifactInput,
	globalVars map[string]string,
	parameters map[string]string,
	requireExistingFiles bool,
) ([]string, error) {
	return artifactResolvePathsContext(
		helmBaseDir,
		items,
		artifactInterpolationContext(globalVars, parameters),
		requireExistingFiles,
	)
}

func artifactResolvePathsContext(
	helmBaseDir string,
	items []ir.HelmArtifactInput,
	ctx expand.InterpolationContext,
	requireExistingFiles bool,
) ([]string, error) {
	pathSet := map[string]struct{}{}
	for _, item := range items {
		paths, err := artifactResolveItemContext(helmBaseDir, item, ctx, requireExistingFiles)
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			pathSet[path] = struct{}{}
		}
	}
	return artifactSortedPaths(pathSet), nil
}

func artifactResolveItemContext(
	helmBaseDir string,
	item ir.HelmArtifactInput,
	ctx expand.InterpolationContext,
	requireExistingFiles bool,
) ([]string, error) {
	switch item.Kind {
	case ir.ArtifactInputString:
		path := expand.InterpolationContextExpandLiteral(ctx, item.Literal)
		if path == "" {
			return nil, nil
		}
		if paths, ok := expand.PathsFromShellParameterList(path); ok {
			return artifactResolveAnchoredPaths(helmBaseDir, paths, requireExistingFiles)
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
		glob := ArtifactGlobWithInterpolatedBaseContext(item.Glob, ctx)
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
	return ArtifactGlobWithInterpolatedBaseContext(
		glob,
		artifactInterpolationContext(globalVars, parameters),
	)
}

func ArtifactGlobWithInterpolatedBaseContext(
	glob *ir.HelmGlob,
	ctx expand.InterpolationContext,
) *ir.HelmGlob {
	if glob == nil {
		return nil
	}
	copy := *glob
	copy.BaseDirectory = expand.InterpolationContextExpandLiteral(ctx, glob.BaseDirectory)
	copy.Includes = expandInterpolateGlobPatternsContext(glob.Includes, ctx)
	copy.Excludes = expandInterpolateGlobPatternsContext(glob.Excludes, ctx)
	copy.Types = expand.InterpolationContextExpandLiteral(ctx, glob.Types)
	return &copy
}

func expandInterpolateGlobPatternsContext(
	patterns []string,
	ctx expand.InterpolationContext,
) []string {
	if len(patterns) == 0 {
		return nil
	}
	out := make([]string, len(patterns))
	for i, pattern := range patterns {
		out[i] = expand.InterpolationContextExpandLiteral(ctx, pattern)
	}
	return out
}

// ArtifactAnchorPath resolves a helm-relative path against the helm file directory.
func ArtifactAnchorPath(helmBaseDir, path string) string {
	return artifactAnchorPath(helmBaseDir, path)
}

func artifactAnchorPath(helmBaseDir, path string) string {
	if path == "" || helmBaseDir == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(helmBaseDir, path)
}

func artifactResolveAnchoredPaths(
	helmBaseDir string,
	paths []string,
	requireExistingFiles bool,
) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = artifactAnchorPath(helmBaseDir, path)
		if requireExistingFiles {
			if err := artifactEnsureFile(path); err != nil {
				return nil, err
			}
		}
		out = append(out, path)
	}
	return out, nil
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
