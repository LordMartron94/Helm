package artifactresolve

import (
	"fmt"
	"foundation/system"
	"helm/internal/ir"
	"os"
	"path/filepath"
	"strings"
)

func ArtifactWalkGlob(helmBaseDir string, glob *ir.HelmGlob) ([]string, error) {
	if glob == nil {
		return nil, fmt.Errorf("glob configuration is nil")
	}

	baseDir := artifactAnchorPath(helmBaseDir, filepath.Clean(glob.BaseDirectory))
	if baseDir == "" {
		return nil, fmt.Errorf("glob base directory is empty")
	}

	include := glob.Include
	if include == "" {
		include = "**"
	}

	pattern := include
	if !strings.Contains(pattern, string(filepath.Separator)) && !strings.HasPrefix(pattern, "**") {
		pattern = "**" + string(filepath.Separator) + pattern
	}

	candidates, err := filepath.Glob(filepath.Join(baseDir, pattern))
	if err != nil {
		return nil, fmt.Errorf("glob include pattern %q: %w", include, err)
	}

	pathSet := map[string]struct{}{}
	for _, candidate := range candidates {
		if err := artifactCollectGlobCandidate(glob, baseDir, candidate, pathSet); err != nil {
			return nil, err
		}
	}

	if !glob.Recursive {
		topLevel := map[string]struct{}{}
		for path := range pathSet {
			rel, err := filepath.Rel(baseDir, path)
			if err != nil {
				continue
			}
			if !strings.Contains(rel, string(filepath.Separator)) {
				topLevel[path] = struct{}{}
			}
		}
		pathSet = topLevel
	}

	return artifactSortedPaths(pathSet), nil
}

func artifactCollectGlobCandidate(
	glob *ir.HelmGlob,
	baseDir string,
	candidate string,
	pathSet map[string]struct{},
) error {
	info, err := os.Stat(candidate)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	rel, err := filepath.Rel(baseDir, candidate)
	if err != nil {
		return err
	}
	rel = filepath.ToSlash(rel)
	if artifactPathMatchesExclude(rel, glob.Exclude) {
		return nil
	}

	switch glob.Types {
	case "directories":
		if !info.IsDir() {
			return nil
		}
	case "files":
		if info.IsDir() {
			if glob.Recursive {
				return artifactWalkDirectoryFiles(glob, candidate, pathSet)
			}
			return nil
		}
	default:
		if info.IsDir() {
			if glob.Recursive {
				return artifactWalkDirectoryFiles(glob, candidate, pathSet)
			}
			return nil
		}
	}

	pathSet[candidate] = struct{}{}
	return nil
}

func artifactWalkDirectoryFiles(
	glob *ir.HelmGlob,
	dir string,
	pathSet map[string]struct{},
) error {
	return system.ScanRecursive(dir, func(path string, name string, isDir bool) (bool, error) {
		if isDir {
			return false, nil
		}
		rel, err := filepath.Rel(filepath.Clean(glob.BaseDirectory), path)
		if err != nil {
			return false, err
		}
		rel = filepath.ToSlash(rel)
		if artifactPathMatchesExclude(rel, glob.Exclude) {
			return false, nil
		}
		pathSet[path] = struct{}{}
		return false, nil
	})
}
