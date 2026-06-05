package targetexecutor

import (
	"helm/internal/artifactresolve"
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
	resolvedParams, err := TargetExecutorResolveInvocationParameters(
		helmBaseDir,
		globals,
		paramValues,
	)
	if err != nil {
		return "", nil, err
	}

	globalVars, err := TargetExecutorInterpolationGlobals(helmBaseDir, globals, resolvedParams)
	if err != nil {
		return "", nil, err
	}

	return targetExecutorResolveTargetRunsFromResolved(
		helmBaseDir,
		target,
		globals,
		paramValues,
		globalVars,
		resolvedParams,
	)
}

func targetExecutorResolveTargetRunsFromResolved(
	helmBaseDir string,
	target ir.HelmTarget,
	globals map[string]ir.HelmGlobalVariable,
	paramValues map[string]ir.HelmParameterValue,
	globalVars map[string]string,
	resolvedParams map[string]string,
) (workDir string, steps []TargetResolvedRun, err error) {
	workDir = TargetExecutorInterpolateLiteral(target.WorkDir, globalVars, resolvedParams)
	workDir = artifactresolve.ArtifactAnchorPath(helmBaseDir, workDir)
	if workDir == "" {
		workDir = helmBaseDir
	}

	steps, err = targetExecutorResolveTargetRunSteps(
		helmBaseDir,
		target,
		globals,
		paramValues,
		globalVars,
		resolvedParams,
	)
	if err != nil {
		return "", nil, err
	}

	return workDir, steps, nil
}
