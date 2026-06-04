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
	parameters map[string]string,
) (map[string]string, error) {
	scalarGlobals := ir.InterpolationGlobalsFromHelmGlobals(globals)
	if len(globals) == 0 {
		return scalarGlobals, nil
	}

	merged := make(map[string]string, len(globals))
	for name, value := range scalarGlobals {
		merged[name] = value
	}

	for name, variable := range globals {
		if variable.Kind != ir.HelmGlobalVarArtifactArray {
			continue
		}

		paths, err := artifactresolve.ArtifactResolveItems(
			helmBaseDir,
			variable.ArtifactItems,
			merged,
			parameters,
			false,
		)
		if err != nil {
			return nil, fmt.Errorf("variable '%s': %w", name, err)
		}

		merged[name] = expand.JoinPathsForShell(paths)
	}

	return merged, nil
}
