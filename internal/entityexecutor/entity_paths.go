package entityexecutor

import (
	"path/filepath"

	"helm/internal/ir"
	"helm/internal/workspacepath"
)

// EntityDeclarationBaseDir returns the directory containing the helm file that declared entity.
func EntityDeclarationBaseDir(workspaceRoot string, entity ir.HelmEntity) string {
	if entity.SourceFile != "" {
		return filepath.Dir(entity.SourceFile)
	}
	return workspaceRoot
}

// EntityPathsToWorkspace maps paths resolved relative to declarationBase into workspace-root paths.
func EntityPathsToWorkspace(workspaceRoot, declarationBase string, paths []string) []string {
	if len(paths) == 0 || declarationBase == "" || workspaceRoot == "" {
		return paths
	}
	declAbs, err := filepath.Abs(declarationBase)
	if err != nil {
		return paths
	}
	rootAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return paths
	}
	if declAbs == rootAbs {
		return paths
	}

	out := make([]string, 0, len(paths))
	for _, path := range paths {
		abs := filepath.Join(declAbs, filepath.FromSlash(path))
		rel, ok := workspacepath.WorkspaceRelative(rootAbs, abs)
		if ok {
			out = append(out, rel)
			continue
		}
		out = append(out, workspacepath.WorkspaceNormalize(path))
	}
	return out
}
