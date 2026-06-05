package targetexecutor

import (
	"fmt"
	"helm/internal/artifactresolve"
	"helm/internal/ir"
)

// TargetExecutorResolveTargetRuns expands a target's steps into concrete command strings
// and an anchored working directory (helm file directory when workdir is unset).
func TargetExecutorResolveTargetRuns(
	helmBaseDir string,
	target ir.HelmTarget,
	globals map[string]ir.HelmGlobalVariable,
	inv TargetInvocation,
) (workDir string, commands []string, err error) {
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
		globalVars,
		resolvedParams,
	)
}

func targetExecutorResolveTargetRunsFromResolved(
	helmBaseDir string,
	target ir.HelmTarget,
	globalVars map[string]string,
	resolvedParams map[string]string,
) (workDir string, commands []string, err error) {
	workDir = TargetExecutorInterpolateLiteral(target.WorkDir, globalVars, resolvedParams)
	workDir = artifactresolve.ArtifactAnchorPath(helmBaseDir, workDir)
	if workDir == "" {
		workDir = helmBaseDir
	}

	for _, step := range target.Steps {
		switch step.Kind {
		case ir.TargetStepRun:
			commands = append(
				commands,
				TargetExecutorResolveRunCommand(step.Run, globalVars, resolvedParams),
			)
		case ir.TargetStepWhen:
			if step.When == nil {
				continue
			}
			if !TargetExecutorEvaluateCondition(*step.When, resolvedParams) {
				continue
			}
			for _, runLiteral := range step.When.Runs {
				commands = append(
					commands,
					TargetExecutorResolveRunCommand(runLiteral, globalVars, resolvedParams),
				)
			}
		default:
			return "", nil, fmt.Errorf("target '%s': unknown step kind", target.Name)
		}
	}

	return workDir, commands, nil
}
