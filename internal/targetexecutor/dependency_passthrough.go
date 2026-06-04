package targetexecutor

import (
	"helm/internal/artifactresolve"
	"helm/internal/cache"
	"helm/internal/ir"
	"os"
	"path/filepath"
)

func targetExecutorPassthroughFingerprintError(
	builtIR ir.HelmIR,
	dependentCanonical string,
	err error,
	results map[string]error,
	nodeInvocations map[string]TargetInvocation,
) error {
	if err == nil {
		return nil
	}

	missingPath, ok := cache.CacheFingerprintFileErrorPath(err)
	if !ok || !cache.CacheFingerprintFileErrorIsNotExist(err) {
		return err
	}

	if blocked := targetExecutorDependencyBlockedByFailedArtifact(
		builtIR,
		dependentCanonical,
		missingPath,
		results,
		nodeInvocations,
	); blocked != nil {
		return blocked
	}

	return err
}

func targetExecutorDependencyBlockedByFailedArtifact(
	builtIR ir.HelmIR,
	dependentCanonical string,
	missingPath string,
	results map[string]error,
	nodeInvocations map[string]TargetInvocation,
) error {
	cleanMissing := filepath.Clean(missingPath)

	for execNodeID, depErr := range results {
		if depErr == nil {
			continue
		}
		if !targetExecutorFailedNodeReferencesArtifactPath(
			builtIR,
			execNodeID,
			nodeInvocations[execNodeID],
			cleanMissing,
		) {
			continue
		}

		depCanonical := TargetExecutorExecutionNodeCanonical(execNodeID)
		return TargetExecutorFormatDependencyBlockedError(dependentCanonical, depCanonical, depErr)
	}

	return nil
}

func targetExecutorFailedNodeReferencesArtifactPath(
	builtIR ir.HelmIR,
	execNodeID string,
	inv TargetInvocation,
	artifactPath string,
) bool {
	canonical := TargetExecutorExecutionNodeCanonical(execNodeID)
	target, ok := builtIR.Targets[canonical]
	if !ok || target.Artifacts == nil {
		return false
	}

	globalVars, parameters, err := targetExecutorCacheInterpolationGlobals(builtIR, target, inv)
	if err != nil {
		return false
	}

	outputPaths, err := artifactresolve.ArtifactResolveOutputPaths(
		builtIR.SourceDirectory,
		target.Artifacts.Outputs,
		globalVars,
		parameters,
	)
	if err != nil {
		return false
	}
	if targetExecutorPathsContain(outputPaths, artifactPath) {
		return true
	}

	inputPaths, err := artifactresolve.ArtifactResolveInputPaths(
		builtIR.SourceDirectory,
		target.Artifacts,
		globalVars,
		parameters,
	)
	if err != nil {
		return false
	}

	return targetExecutorPathsContain(inputPaths, artifactPath)
}

func targetExecutorPathsContain(paths []string, artifactPath string) bool {
	for _, path := range paths {
		if filepath.Clean(path) == artifactPath {
			return true
		}
	}
	return false
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

func targetExecutorWrapCacheError(
	builtIR ir.HelmIR,
	targetName string,
	err error,
	results map[string]error,
	nodeInvocations map[string]TargetInvocation,
) error {
	if err == nil {
		return nil
	}
	return targetExecutorPassthroughFingerprintError(
		builtIR,
		targetName,
		err,
		results,
		nodeInvocations,
	)
}

func targetExecutorDependencyBlockedTransitive(
	plan *TargetExecutionPlan,
	builtIR ir.HelmIR,
	executionNodeID string,
	results map[string]error,
) error {
	canonicalName := TargetExecutorExecutionNodeCanonical(executionNodeID)
	target := builtIR.Targets[canonicalName]

	depNodes := plan.Outgoing[canonicalName]
	if len(depNodes) != len(target.DependsOn) {
		depNodes = targetExecutorFallbackDependencyNodes(builtIR, target)
	}

	visited := make(map[string]struct{})
	for i, dep := range target.DependsOn {
		depNode := dep.TargetName
		if i < len(depNodes) {
			depNode = depNodes[i]
		}

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

	depCanonical := TargetExecutorExecutionNodeCanonical(depNode)
	depTarget := builtIR.Targets[depCanonical]

	depNodes := plan.Outgoing[depCanonical]
	if len(depNodes) != len(depTarget.DependsOn) {
		depNodes = targetExecutorFallbackDependencyNodes(builtIR, depTarget)
	}

	for i, dep := range depTarget.DependsOn {
		childNode := dep.TargetName
		if i < len(depNodes) {
			childNode = depNodes[i]
		}

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

func targetExecutorDependencyBlocked(
	plan *TargetExecutionPlan,
	builtIR ir.HelmIR,
	executionNodeID string,
	results map[string]error,
) error {
	return targetExecutorDependencyBlockedTransitive(plan, builtIR, executionNodeID, results)
}
