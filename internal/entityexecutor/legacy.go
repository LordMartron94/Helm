package entityexecutor

import "helm/internal/ir"

// LegacyAdapterMode reports whether Helm should route artifact builds through the v1.7
// target executor only. The interpreter delegates to targetexecutor when this is true.
func LegacyAdapterMode(builtIR ir.HelmIR) bool {
	return EntityExecutorLegacyMode(builtIR)
}

// EntityCollectAllRoots returns every entity label in the workspace (for full export).
func EntityCollectAllRoots(builtIR ir.HelmIR) []ir.HelmLabel {
	roots := make([]ir.HelmLabel, 0, len(builtIR.Entities))
	for _, entity := range builtIR.Entities {
		roots = append(roots, entity.Label)
	}
	return roots
}
