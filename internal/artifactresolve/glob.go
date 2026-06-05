package artifactresolve

import (
	"fmt"
	"foundation/system"
	"helm/internal/ir"
	"helm/internal/workspacepath"
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

	includes := glob.Includes
	if len(includes) == 0 {
		includes = []string{"**"}
	}

	pathSet := map[string]struct{}{}

	err := system.ScanRecursive(baseDir, func(path string, name string, isDir bool) (bool, error) {
		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return false, err
		}
		rel = filepath.ToSlash(rel)

		if artifactPathMatchesAnyExclude(rel, name, glob.Excludes) {
			if isDir && artifactExcludePrunesSubtree(rel, glob.Excludes) {
				return true, nil
			}
			return false, nil
		}

		if !glob.Recursive {
			if isDir {
				if glob.Types == "directories" && artifactPathMatchesAnyInclude(rel, name, includes) {
					artifactGlobStoreWorkspacePath(pathSet, helmBaseDir, path)
				}
				return true, nil
			}
			if strings.Contains(rel, "/") {
				return false, nil
			}
		}

		switch glob.Types {
		case "directories":
			if !isDir {
				return false, nil
			}
		case "files":
			if isDir {
				return false, nil
			}
		default:
			if isDir {
				return false, nil
			}
		}

		if artifactPathMatchesAnyInclude(rel, name, includes) {
			artifactGlobStoreWorkspacePath(pathSet, helmBaseDir, path)
		}

		return false, nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to walk glob base directory %q: %w", baseDir, err)
	}

	return artifactSortedPaths(pathSet), nil
}

func artifactGlobStoreWorkspacePath(pathSet map[string]struct{}, helmBaseDir, absolutePath string) {
	rel, ok := workspacepath.WorkspaceRelative(helmBaseDir, absolutePath)
	if !ok {
		return
	}
	pathSet[rel] = struct{}{}
}

func artifactPathMatchesAnyInclude(relPath string, baseName string, includes []string) bool {
	for _, pattern := range includes {
		if artifactPathMatchesInclude(relPath, baseName, pattern) {
			return true
		}
	}
	return false
}

func artifactPathMatchesInclude(relPath string, baseName string, pattern string) bool {
	pattern = filepath.ToSlash(pattern)
	if pattern == "" {
		return false
	}
	if pattern == "**" {
		return true
	}

	if strings.HasPrefix(pattern, "**/") {
		rest := strings.TrimPrefix(pattern, "**/")
		if rest == "" {
			return true
		}
		if !strings.Contains(rest, "/") {
			matched, err := filepath.Match(rest, baseName)
			return err == nil && matched
		}
		return artifactMatchPathAfterGlobstar(relPath, rest)
	}

	matched, err := filepath.Match(pattern, relPath)
	return err == nil && matched
}

func artifactMatchPathAfterGlobstar(relPath string, subpattern string) bool {
	subpattern = filepath.ToSlash(subpattern)
	relPath = filepath.ToSlash(relPath)

	if matched, err := filepath.Match(subpattern, relPath); err == nil && matched {
		return true
	}

	for index := 0; index < len(relPath); index++ {
		if relPath[index] != '/' {
			continue
		}
		tail := relPath[index+1:]
		if matched, err := filepath.Match(subpattern, tail); err == nil && matched {
			return true
		}
	}

	return false
}

func artifactPathMatchesAnyExclude(relPath string, baseName string, excludes []string) bool {
	for _, pattern := range excludes {
		if artifactPathMatchesExclude(relPath, baseName, pattern) {
			return true
		}
	}
	return false
}

func artifactPathMatchesExclude(relPath string, baseName string, exclude string) bool {
	if exclude == "" {
		return false
	}

	exclude = filepath.ToSlash(exclude)
	if artifactExcludePrunesSubtree(relPath, []string{exclude}) {
		return true
	}

	if strings.HasPrefix(exclude, "**/") {
		return artifactPathMatchesInclude(relPath, baseName, exclude)
	}

	matched, err := filepath.Match(exclude, relPath)
	if err == nil && matched {
		return true
	}

	if !strings.Contains(exclude, "/") {
		matched, err = filepath.Match(exclude, baseName)
		return err == nil && matched
	}

	return false
}

func artifactExcludePrunesSubtree(relPath string, excludes []string) bool {
	relPath = filepath.ToSlash(relPath)

	for _, exclude := range excludes {
		exclude = filepath.ToSlash(exclude)
		if exclude == "" {
			continue
		}

		if strings.HasSuffix(exclude, "/**") {
			prefix := strings.TrimSuffix(exclude, "/**")
			if relPath == prefix || strings.HasPrefix(relPath, prefix+"/") {
				return true
			}
			continue
		}

		if strings.HasSuffix(exclude, "/*") {
			prefix := strings.TrimSuffix(exclude, "/*")
			if relPath == prefix || strings.HasPrefix(relPath, prefix+"/") {
				return true
			}
		}
	}

	return false
}
