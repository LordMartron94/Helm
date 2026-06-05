package targetexecutor

import (
	"fmt"
	"helm/internal/expand"
	"helm/internal/ir"
	"helm/internal/workspacepath"
)

func targetExecutorResolveRunArgv(
	helmBaseDir string,
	template []ir.HelmRunArgvElement,
	targets map[string]ir.HelmTarget,
	globals map[string]ir.HelmGlobalVariable,
	paramValues map[string]ir.HelmParameterValue,
	interpCtx expand.InterpolationContext,
	resolved TargetResolvedParameters,
) ([]string, error) {
	if len(template) == 0 {
		return nil, fmt.Errorf("run argv: empty command array")
	}

	argv := make([]string, 0, len(template))
	for _, element := range template {
		if element.Collect != nil {
			fragment, err := expand.EvaluateStringListExpr(
				ir.HelmStringListExpr{{
					Kind:    ir.StringListCollect,
					Collect: element.Collect,
				}},
				targets,
				paramValues,
				resolved.Scalars,
				resolved.StringLists,
				interpCtx,
			)
			if err != nil {
				return nil, err
			}
			argv = append(argv, fragment...)
			continue
		}
		if element.ParamName != "" {
			fragment, err := targetExecutorRunArgvParameterFragment(
				helmBaseDir,
				element.ParamName,
				targets,
				globals,
				paramValues,
				interpCtx,
				resolved,
			)
			if err != nil {
				return nil, err
			}
			argv = append(argv, fragment...)
			continue
		}
		if element.AbsPath != "" {
			rel := expand.InterpolationContextExpandLiteral(interpCtx, element.AbsPath)
			argv = append(argv, workspacepath.WorkspaceAnchor(helmBaseDir, rel))
			continue
		}

		argv = append(argv, expand.InterpolationContextExpandLiteral(interpCtx, element.Literal))
	}

	if len(argv) == 0 {
		return nil, fmt.Errorf("run argv: command array produced no arguments")
	}

	return argv, nil
}

func targetExecutorRunArgvParameterFragment(
	helmBaseDir string,
	paramName string,
	targets map[string]ir.HelmTarget,
	globals map[string]ir.HelmGlobalVariable,
	paramValues map[string]ir.HelmParameterValue,
	interpCtx expand.InterpolationContext,
	resolved TargetResolvedParameters,
) ([]string, error) {
	if fragments, ok := resolved.StringLists[paramName]; ok {
		return append([]string(nil), fragments...), nil
	}
	if paths, ok := interpCtx.PathLists[paramName]; ok {
		return append([]string(nil), paths...), nil
	}
	if scalar, ok := resolved.Scalars[paramName]; ok {
		if scalar == "" {
			return nil, nil
		}
		return []string{scalar}, nil
	}
	return targetExecutorResolveParameterPaths(
		helmBaseDir,
		paramName,
		globals,
		paramValues,
		interpCtx,
	)
}

func targetExecutorRunArgvPathsFromGlobal(
	helmBaseDir string,
	globals map[string]ir.HelmGlobalVariable,
	globalName string,
	interpCtx expand.InterpolationContext,
) ([]string, error) {
	return targetExecutorResolveGlobalArtifactPaths(
		helmBaseDir,
		globals,
		globalName,
		interpCtx,
	)
}
