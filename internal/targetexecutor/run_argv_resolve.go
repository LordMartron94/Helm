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
	globals map[string]ir.HelmGlobalVariable,
	paramValues map[string]ir.HelmParameterValue,
	interpCtx expand.InterpolationContext,
) ([]string, error) {
	if len(template) == 0 {
		return nil, fmt.Errorf("run argv: empty command array")
	}

	argv := make([]string, 0, len(template))
	for _, element := range template {
		if element.ParamName != "" {
			paths, err := targetExecutorRunArgvParameterPaths(
				helmBaseDir,
				element.ParamName,
				globals,
				paramValues,
				interpCtx,
			)
			if err != nil {
				return nil, err
			}
			argv = append(argv, paths...)
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

func targetExecutorRunArgvParameterPaths(
	helmBaseDir string,
	paramName string,
	globals map[string]ir.HelmGlobalVariable,
	paramValues map[string]ir.HelmParameterValue,
	interpCtx expand.InterpolationContext,
) ([]string, error) {
	if paths, ok := interpCtx.PathLists[paramName]; ok {
		return append([]string(nil), paths...), nil
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
