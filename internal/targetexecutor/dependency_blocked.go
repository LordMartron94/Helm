package targetexecutor

import (
	"fmt"
	"helm/internal/artifactresolve"
	"helm/internal/ir"
	"os"
)

func targetExecutorDependencyNodes(
	plan *TargetExecutionPlan,
	builtIR ir.HelmIR,
	executionNodeID string,
	inv TargetInvocation,
) ([]string, error) {
	canonical := TargetExecutorExecutionNodeCanonical(executionNodeID)
	target, ok := builtIR.Targets[canonical]
	if !ok {
		return nil, fmt.Errorf("target '%s' does not exist in IR", canonical)
	}

	for _, key := range []string{executionNodeID, canonical} {
		depNodes, exists := plan.Outgoing[key]
		if exists && len(depNodes) == len(target.DependsOn) {
			return depNodes, nil
		}
	}

	parentResolved, err := targetExecutorResolvedParamsForExecutionNode(
		builtIR,
		executionNodeID,
		inv,
	)
	if err != nil {
		return nil, fmt.Errorf("target '%s': %w", canonical, err)
	}

	out := make([]string, len(target.DependsOn))
	for i, dep := range target.DependsOn {
		depCanonical, ok := ir.IRResolveTargetName(builtIR.Targets, dep.TargetName)
		if !ok {
			return nil, fmt.Errorf(
				"target '%s' depends on undeclared target '%s'",
				canonical,
				dep.TargetName,
			)
		}

		if len(dep.Parameters) == 0 {
			out[i] = depCanonical
			continue
		}

		bound, bindErr := targetExecutorBindDependencyParams(
			builtIR.GlobalVariables,
			parentResolved,
			dep.Parameters,
		)
		if bindErr != nil {
			return nil, fmt.Errorf("target '%s' dependency '%s': %w", canonical, dep.TargetName, bindErr)
		}

		out[i] = targetExecutorParametricInstanceID(depCanonical, bound)
	}

	return out, nil
}

func targetExecutorResolvedParamsForExecutionNode(
	builtIR ir.HelmIR,
	executionNodeID string,
	inv TargetInvocation,
) (map[string]string, error) {
	canonical := TargetExecutorExecutionNodeCanonical(executionNodeID)
	target := builtIR.Targets[canonical]
	paramValues := TargetExecutorParametersForTarget(target, inv)
	return TargetExecutorResolveInvocationParameters(
		builtIR.SourceDirectory,
		builtIR.GlobalVariables,
		paramValues,
	)
}

func targetExecutorDependencyBlocked(
	plan *TargetExecutionPlan,
	builtIR ir.HelmIR,
	executionNodeID string,
	inv TargetInvocation,
	results map[string]error,
) error {
	canonicalName := TargetExecutorExecutionNodeCanonical(executionNodeID)

	depNodes, err := targetExecutorDependencyNodes(plan, builtIR, executionNodeID, inv)
	if err != nil {
		return err
	}

	target := builtIR.Targets[canonicalName]
	visited := make(map[string]struct{})
	for i, dep := range target.DependsOn {
		depNode := depNodes[i]
		if blocked := targetExecutorDependencyBlockedVisit(
			plan,
			builtIR,
			canonicalName,
			depNode,
			dep.Optional,
			results,
			visited,
		); blocked != nil {
			return blocked
		}
	}

	return nil
}

func targetExecutorDependencyBlockedVisit(
	plan *TargetExecutionPlan,
	builtIR ir.HelmIR,
	dependentCanonical string,
	depNode string,
	optional bool,
	results map[string]error,
	visited map[string]struct{},
) error {
	if _, seen := visited[depNode]; seen {
		return nil
	}
	visited[depNode] = struct{}{}

	depErr := results[depNode]
	if depErr != nil && !optional {
		depCanonical := TargetExecutorExecutionNodeCanonical(depNode)
		return TargetExecutorFormatDependencyBlockedError(dependentCanonical, depCanonical, depErr)
	}

	childInv := targetExecutorInvocationForNode(plan, depNode, nil)
	childNodes, err := targetExecutorDependencyNodes(plan, builtIR, depNode, childInv)
	if err != nil {
		return err
	}

	depCanonical := TargetExecutorExecutionNodeCanonical(depNode)
	depTarget := builtIR.Targets[depCanonical]

	for i, dep := range depTarget.DependsOn {
		childNode := childNodes[i]
		if blocked := targetExecutorDependencyBlockedVisit(
			plan,
			builtIR,
			dependentCanonical,
			childNode,
			dep.Optional,
			results,
			visited,
		); blocked != nil {
			return blocked
		}
	}

	return nil
}

func targetExecutorCacheHitOutputsExist(
	builtIR ir.HelmIR,
	target ir.HelmTarget,
	inv TargetInvocation,
) (bool, error) {
	if target.Artifacts == nil || len(target.Artifacts.Outputs) == 0 {
		return true, nil
	}

	globalVars, parameters, err := targetExecutorCacheInterpolationGlobals(builtIR, target, inv)
	if err != nil {
		return false, err
	}

	outputPaths, err := artifactresolve.ArtifactResolveOutputPaths(
		builtIR.SourceDirectory,
		target.Artifacts.Outputs,
		globalVars,
		parameters,
	)
	if err != nil {
		return false, err
	}

	for _, path := range outputPaths {
		if _, statErr := os.Stat(path); statErr != nil {
			if os.IsNotExist(statErr) {
				return false, nil
			}
			return false, statErr
		}
	}

	return true, nil
}

func targetExecutorInvalidateStaleCacheHit(
	builtIR ir.HelmIR,
	target ir.HelmTarget,
	inv TargetInvocation,
	decision *targetExecutorCacheDecision,
) error {
	if !decision.Skip {
		return nil
	}

	outputsExist, err := targetExecutorCacheHitOutputsExist(builtIR, target, inv)
	if err != nil {
		return err
	}
	if outputsExist {
		return nil
	}

	decision.Skip = false
	decision.OutputFingerprint = 0
	return nil
}
