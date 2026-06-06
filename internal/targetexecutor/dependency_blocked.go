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
		if exists {
			parentResolved, resolveErr := targetExecutorResolvedParamsForExecutionNode(
				builtIR,
				executionNodeID,
				inv,
			)
			if resolveErr != nil {
				return nil, resolveErr
			}
			effectiveDeps, expandErr := targetExecutorEffectiveDependsOn(
				builtIR,
				target,
				inv,
				parentResolved,
			)
			if expandErr != nil {
				return nil, expandErr
			}
			if len(depNodes) == len(effectiveDeps) {
				return depNodes, nil
			}
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

	effectiveDeps, err := targetExecutorEffectiveDependsOn(
		builtIR,
		target,
		inv,
		parentResolved,
	)
	if err != nil {
		return nil, err
	}

	return targetExecutorDependencyNodesFromEdges(builtIR, canonical, effectiveDeps)
}

func targetExecutorResolvedParamsForExecutionNode(
	builtIR ir.HelmIR,
	executionNodeID string,
	inv TargetInvocation,
) (TargetResolvedParameters, error) {
	canonical := TargetExecutorExecutionNodeCanonical(executionNodeID)
	target := builtIR.Targets[canonical]
	paramValues := TargetExecutorParametersForTarget(target, inv)
	return TargetExecutorResolveInvocationParameters(
		builtIR.SourceDirectory,
		ir.IRGlobalsForRootManifest(builtIR),
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
	target := builtIR.Targets[canonicalName]

	parentResolved, err := targetExecutorResolvedParamsForExecutionNode(
		builtIR,
		executionNodeID,
		inv,
	)
	if err != nil {
		return err
	}

	effectiveDeps, err := targetExecutorEffectiveDependsOn(
		builtIR,
		target,
		inv,
		parentResolved,
	)
	if err != nil {
		return err
	}

	visited := make(map[string]struct{})
	for _, dep := range effectiveDeps {
		if ir.HelmTargetDependencyIsEntityLabel(dep) {
			continue
		}
		depNode, nodeErr := targetExecutorDependencyNodeFromEdge(builtIR, dep)
		if nodeErr != nil {
			return nodeErr
		}

		if blocked := targetExecutorDependencyBlockedVisit(
			plan,
			builtIR,
			canonicalName,
			depNode,
			dep.Optional,
			inv,
			results,
			visited,
		); blocked != nil {
			return blocked
		}
	}

	return nil
}

func targetExecutorDependencyNodeFromEdge(
	builtIR ir.HelmIR,
	dep ir.HelmTargetDependency,
) (string, error) {
	if ir.HelmTargetDependencyIsEntityLabel(dep) {
		return "", fmt.Errorf("entity label dependency %q is not a target execution node", dep.TargetName)
	}
	nodes, err := targetExecutorDependencyNodesFromEdges(builtIR, dep.TargetName, []ir.HelmTargetDependency{dep})
	if err != nil {
		return "", err
	}
	if len(nodes) == 0 {
		return "", fmt.Errorf("dependency '%s' produced no execution nodes", dep.TargetName)
	}
	return nodes[0], nil
}

func targetExecutorDependencyBlockedVisit(
	plan *TargetExecutionPlan,
	builtIR ir.HelmIR,
	dependentCanonical string,
	depNode string,
	optional bool,
	inv TargetInvocation,
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
	childCanonical := TargetExecutorExecutionNodeCanonical(depNode)
	childTarget := builtIR.Targets[childCanonical]

	childResolved, err := targetExecutorResolvedParamsForExecutionNode(builtIR, depNode, childInv)
	if err != nil {
		return err
	}

	childDeps, err := targetExecutorEffectiveDependsOn(
		builtIR,
		childTarget,
		childInv,
		childResolved,
	)
	if err != nil {
		return err
	}

	for _, dep := range childDeps {
		if ir.HelmTargetDependencyIsEntityLabel(dep) {
			continue
		}
		childNode, nodeErr := targetExecutorDependencyNodeFromEdge(builtIR, dep)
		if nodeErr != nil {
			return nodeErr
		}

		if blocked := targetExecutorDependencyBlockedVisit(
			plan,
			builtIR,
			dependentCanonical,
			childNode,
			dep.Optional,
			childInv,
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

	interpCtx, err := targetExecutorCacheInterpolationGlobals(builtIR, target, inv)
	if err != nil {
		return false, err
	}

	outputPaths, err := artifactresolve.ArtifactResolveOutputPathsContext(
		builtIR.SourceDirectory,
		target.Artifacts.Outputs,
		interpCtx,
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
