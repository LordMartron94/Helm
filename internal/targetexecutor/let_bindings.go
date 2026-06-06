package targetexecutor

import (
	"fmt"
	"helm/internal/expand"
	"helm/internal/ir"
	"helm/internal/pathexpr"
)

func TargetExecutorEvaluateLetBindings(
	helmBaseDir string,
	target ir.HelmTarget,
	interpCtx expand.InterpolationContext,
) (expand.InterpolationContext, error) {
	if len(target.LetBindings) == 0 {
		return interpCtx, nil
	}

	scope := pathexpr.PathExprScopeFromInterpolation(interpCtx)
	if interpCtx.PathLists == nil {
		interpCtx.PathLists = make(map[string][]string)
	}

	for _, binding := range target.LetBindings {
		paths, err := pathexpr.PathExprEvaluate(helmBaseDir, binding.Expr, scope)
		if err != nil {
			return expand.InterpolationContext{}, fmt.Errorf(
				"target '%s' let '%s': %w",
				target.Name,
				binding.Name,
				err,
			)
		}
		interpCtx.PathLists[binding.Name] = paths
		scope.PathLists[binding.Name] = append([]string(nil), paths...)
	}

	return interpCtx, nil
}

func TargetExecutorInterpolationContext(
	helmBaseDir string,
	target ir.HelmTarget,
	globals map[string]ir.HelmGlobalVariable,
	inv TargetInvocation,
) (expand.InterpolationContext, error) {
	paramValues := TargetExecutorParametersForTarget(target, inv)
	resolved, err := TargetExecutorResolveInvocationParameters(
		helmBaseDir,
		globals,
		paramValues,
	)
	if err != nil {
		return expand.InterpolationContext{}, err
	}

	interpCtx, err := TargetExecutorInterpolationGlobals(helmBaseDir, globals, resolved)
	if err != nil {
		return expand.InterpolationContext{}, err
	}

	return TargetExecutorEvaluateLetBindings(helmBaseDir, target, interpCtx)
}
