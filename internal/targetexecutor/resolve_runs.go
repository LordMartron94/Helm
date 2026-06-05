package targetexecutor

import (
	"helm/internal/artifactresolve"
	"helm/internal/expand"
	"helm/internal/ir"
)

// TargetExecutorResolveTargetRuns expands a target's steps into concrete run steps
// and an anchored working directory (helm file directory when workdir is unset).
func TargetExecutorResolveTargetRuns(
	helmBaseDir string,
	target ir.HelmTarget,
	globals map[string]ir.HelmGlobalVariable,
	inv TargetInvocation,
) (workDir string, steps []TargetResolvedRun, err error) {
	paramValues := TargetExecutorParametersForTarget(target, inv)
	resolved, err := TargetExecutorResolveInvocationParameters(
		helmBaseDir,
		globals,
		paramValues,
	)
	if err != nil {
		return "", nil, err
	}

	baseInterpCtx, err := TargetExecutorInterpolationGlobals(helmBaseDir, globals, resolved)
	if err != nil {
		return "", nil, err
	}
	interpCtx, err := TargetExecutorEvaluateLetBindings(helmBaseDir, target, baseInterpCtx)
	if err != nil {
		return "", nil, err
	}

	return targetExecutorResolveTargetRunsFromResolved(
		helmBaseDir,
		target,
		globals,
		paramValues,
		interpCtx,
		resolved,
	)
}

func targetExecutorResolveTargetRunsFromResolved(
	helmBaseDir string,
	target ir.HelmTarget,
	globals map[string]ir.HelmGlobalVariable,
	paramValues map[string]ir.HelmParameterValue,
	interpCtx expand.InterpolationContext,
	resolved TargetResolvedParameters,
) (workDir string, steps []TargetResolvedRun, err error) {
	workDir = TargetExecutorInterpolateLiteral(interpCtx, target.WorkDir)
	workDir = artifactresolve.ArtifactAnchorPath(helmBaseDir, workDir)
	if workDir == "" {
		workDir = helmBaseDir
	}

	steps, err = targetExecutorResolveTargetRunSteps(
		helmBaseDir,
		target,
		globals,
		paramValues,
		interpCtx,
		resolved,
	)
	if err != nil {
		return "", nil, err
	}

	return workDir, steps, nil
}
