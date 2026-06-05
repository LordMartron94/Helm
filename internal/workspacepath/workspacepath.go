package workspacepath

import (
	"path/filepath"
	"strings"
)

// WorkspaceAnchor resolves a workspace-relative path against the helm file directory.
func WorkspaceAnchor(helmRoot, relPath string) string {
	if relPath == "" {
		return relPath
	}
	if helmRoot == "" || filepath.IsAbs(relPath) {
		return relPath
	}
	return filepath.Join(helmRoot, relPath)
}

// WorkspaceNormalize returns a clean workspace-relative path with forward slashes.
func WorkspaceNormalize(relPath string) string {
	if relPath == "" {
		return relPath
	}
	return filepath.ToSlash(filepath.Clean(relPath))
}

// WorkspaceRelative maps an absolute or helm-relative path to a workspace-relative path.
// ok is false when the path is outside helmRoot.
func WorkspaceRelative(helmRoot, path string) (rel string, ok bool) {
	if path == "" || helmRoot == "" {
		return WorkspaceNormalize(path), path != ""
	}

	absPath := path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(helmRoot, path)
	}

	absRoot, err := filepath.Abs(helmRoot)
	if err != nil {
		return "", false
	}
	absPath, err = filepath.Abs(absPath)
	if err != nil {
		return "", false
	}

	rel, err = filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}

	return WorkspaceNormalize(rel), true
}

// WorkspaceContains reports whether path resolves strictly inside helmRoot.
func WorkspaceContains(helmRoot, path string) bool {
	_, ok := WorkspaceRelative(helmRoot, path)
	return ok
}
