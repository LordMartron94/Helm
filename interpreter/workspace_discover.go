package interpreter

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// WorkspaceDiscoverHelmFiles walks workspaceRoot for *.helm files, skipping the root
// manifest and any paths matched by excludes (directories or individual helm files).
func WorkspaceDiscoverHelmFiles(
	workspaceRoot, rootManifest string,
	excludes []string,
) ([]string, error) {
	rootManifestAbs, err := filepath.Abs(rootManifest)
	if err != nil {
		return nil, err
	}

	excludeDirs, excludeFiles := workspaceExcludeSets(excludes)

	var discovered []string
	walkErr := filepath.Walk(workspaceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, relErr := filepath.Rel(workspaceRoot, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}

		if info.IsDir() {
			if workspaceExcludeDirMatches(rel, excludeDirs) {
				return filepath.SkipDir
			}
			return nil
		}

		if filepath.Ext(path) != ".helm" {
			return nil
		}

		abs, absErr := filepath.Abs(path)
		if absErr != nil {
			return absErr
		}
		if abs == rootManifestAbs {
			return nil
		}
		if workspaceExcludeFileMatches(rel, excludeFiles) {
			return nil
		}

		discovered = append(discovered, abs)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	sort.Strings(discovered)
	return discovered, nil
}

func workspaceExcludeSets(excludes []string) (dirs map[string]struct{}, files map[string]struct{}) {
	dirs = make(map[string]struct{})
	files = make(map[string]struct{})
	for _, entry := range excludes {
		entry = strings.TrimSpace(filepath.ToSlash(entry))
		if entry == "" || entry == "." {
			continue
		}
		if strings.HasSuffix(entry, ".helm") {
			files[entry] = struct{}{}
			continue
		}
		dirs[entry] = struct{}{}
	}
	return dirs, files
}

func workspaceExcludeDirMatches(rel string, excludeDirs map[string]struct{}) bool {
	for dir := range excludeDirs {
		if rel == dir || strings.HasPrefix(rel, dir+"/") {
			return true
		}
	}
	return false
}

func workspaceExcludeFileMatches(rel string, excludeFiles map[string]struct{}) bool {
	_, ok := excludeFiles[rel]
	return ok
}
