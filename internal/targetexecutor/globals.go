package targetexecutor

import (
	"fmt"
	"helm/internal/artifactresolve"
	"helm/internal/expand"
	"helm/internal/ir"
)

func TargetExecutorInterpolationGlobals(
	helmBaseDir string,
	globals map[string]ir.HelmGlobalVariable,
	resolved TargetResolvedParameters,
) (expand.InterpolationContext, error) {
	scalarGlobals := ir.InterpolationGlobalsFromHelmGlobals(globals)
	interpCtx := resolved.InterpolationContext(scalarGlobals)

	for name, variable := range globals {
		if variable.Kind != ir.HelmGlobalVarArtifactArray {
			continue
		}
		if _, exists := interpCtx.PathLists[name]; exists {
			continue
		}

		paths, err := artifactresolve.ArtifactResolveItemsContext(
			helmBaseDir,
			variable.ArtifactItems,
			interpCtx,
			false,
		)
		if err != nil {
			return expand.InterpolationContext{}, fmt.Errorf("variable '%s': %w", name, err)
		}

		if interpCtx.PathLists == nil {
			interpCtx.PathLists = make(map[string][]string)
		}
		interpCtx.PathLists[name] = paths
	}

	return interpCtx, nil
}
