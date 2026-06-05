package targetexecutor

import (
	"fmt"
	"helm/internal/artifactresolve"
	"helm/internal/expand"
	"helm/internal/ir"
)

func targetExecutorResolveParameterPaths(
	helmBaseDir string,
	paramName string,
	globals map[string]ir.HelmGlobalVariable,
	paramValues map[string]ir.HelmParameterValue,
	interpCtx expand.InterpolationContext,
) ([]string, error) {
	value, ok := paramValues[paramName]
	if !ok {
		return nil, fmt.Errorf("parameter '%s' is not bound", paramName)
	}

	switch value.Kind {
	case ir.HelmParameterGlobalRef:
		return targetExecutorResolveGlobalArtifactPaths(
			helmBaseDir,
			globals,
			value.GlobalName,
			interpCtx,
		)
	case ir.HelmParameterScalar:
		text := expand.InterpolationContextExpandLiteral(interpCtx, value.Scalar)
		if text == "" {
			return nil, nil
		}
		if paths, ok := expand.PathsFromShellParameterList(text); ok {
			return paths, nil
		}
		return []string{artifactresolve.ArtifactAnchorPath(helmBaseDir, text)}, nil
	case ir.HelmParameterTargetParamRef:
		return nil, fmt.Errorf(
			"parameter '%s' was not bound (internal error)",
			paramName,
		)
	default:
		return nil, fmt.Errorf(
			"parameter '%s' cannot be resolved to artifact paths",
			paramName,
		)
	}
}

func targetExecutorResolveGlobalArtifactPaths(
	helmBaseDir string,
	globals map[string]ir.HelmGlobalVariable,
	globalName string,
	interpCtx expand.InterpolationContext,
) ([]string, error) {
	variable, ok := globals[globalName]
	if !ok {
		return nil, fmt.Errorf("references undeclared global '%s'", globalName)
	}

	switch variable.Kind {
	case ir.HelmGlobalVarArtifactArray:
		return artifactresolve.ArtifactResolveItemsContext(
			helmBaseDir,
			variable.ArtifactItems,
			interpCtx,
			false,
		)
	case ir.HelmGlobalVarString:
		text := expand.InterpolationContextExpandLiteral(interpCtx, variable.StringValue)
		if text == "" {
			return nil, nil
		}
		if paths, ok := expand.PathsFromShellParameterList(text); ok {
			return paths, nil
		}
		return []string{artifactresolve.ArtifactAnchorPath(helmBaseDir, text)}, nil
	default:
		return nil, fmt.Errorf("global '%s' has unsupported kind", globalName)
	}
}
