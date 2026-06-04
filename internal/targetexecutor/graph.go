package targetexecutor

import (
	"fmt"
	"helm/internal/cache"
	"helm/internal/ir"
	"sync"
)

func TargetExecutorRunGraph(
	builtIR ir.HelmIR,
	entryTarget string,
	invocations map[string]TargetInvocation,
	opts TargetExecutorOptions,
) error {
	plan, err := TargetExecutorBuildExecutionPlan(builtIR, entryTarget, invocations)
	if err != nil {
		return err
	}

	canonicalEntry, ok := ir.IRResolveTargetName(builtIR.Targets, entryTarget)
	if !ok {
		return fmt.Errorf("target '%s' does not exist in IR", entryTarget)
	}

	closure := targetExecutorClosureNames(plan.Phases)
	if targetExecutorClosureRequiresConfirm(builtIR, closure) && opts.ConfirmDependency == nil {
		return fmt.Errorf("%s: dependency confirmation required but ConfirmDependency callback is nil", ERROR_CONFIRM_CALLBACK_REQUIRED)
	}

	results := make(map[string]error)
	depStateFingerprints := make(map[string]uint64)
	depOutputFingerprints := make(map[string]uint64)

	runOpts := opts
	var ownedCacheStore *cache.TargetCacheStore
	if !runOpts.DisableArtifactCache && runOpts.CacheStore == nil && runOpts.CacheRoot != "" {
		opened, openErr := cache.TargetCacheStoreOpen(runOpts.CacheRoot)
		if openErr != nil {
			return openErr
		}
		ownedCacheStore = opened
		runOpts.CacheStore = opened
		defer cache.TargetCacheStoreClose(ownedCacheStore)
	}

	for _, phase := range plan.Phases {
		if err := targetExecutorValidatePhaseTTY(phase, builtIR.Targets); err != nil {
			return err
		}

		var wg sync.WaitGroup
		var mu sync.Mutex
		var phaseErr error

		for _, targetName := range phase {
			wg.Add(1)
			go func(name string) {
				defer wg.Done()

				if err := targetExecutorConfirmBeforeTarget(builtIR, TargetExecutorExecutionNodeCanonical(name), closure, runOpts); err != nil {
					mu.Lock()
					if phaseErr == nil {
						phaseErr = err
					}
					mu.Unlock()
					return
				}

				if blockedErr := targetExecutorDependencyBlocked(plan, builtIR, name, results); blockedErr != nil {
					mu.Lock()
					results[name] = blockedErr
					mu.Unlock()
					return
				}

				canonicalName := TargetExecutorExecutionNodeCanonical(name)
				target := builtIR.Targets[canonicalName]
				inv := targetExecutorInvocationForNode(plan, name, invocations)

				paramValues := TargetExecutorParametersForTarget(target, inv)
				resolvedParams, resolveErr := TargetExecutorResolveInvocationParameters(
					builtIR.SourceDirectory,
					builtIR.GlobalVariables,
					paramValues,
				)
				if resolveErr != nil {
					mu.Lock()
					results[name] = resolveErr
					mu.Unlock()
					return
				}
				globalVars, globalErr := TargetExecutorInterpolationGlobals(
					builtIR.SourceDirectory,
					builtIR.GlobalVariables,
					resolvedParams,
				)
				if globalErr != nil {
					mu.Lock()
					results[name] = globalErr
					mu.Unlock()
					return
				}

				instances, instanceErr := TargetExecutorMatrixInstances(
					builtIR.SourceDirectory,
					target,
					globalVars,
					resolvedParams,
				)
				if instanceErr != nil {
					mu.Lock()
					results[name] = instanceErr
					mu.Unlock()
					return
				}

				var instWg sync.WaitGroup
				var instMu sync.Mutex
				var instStateFingerprints []uint64
				var instOutputFingerprints []uint64
				var targetErr error

				for _, instance := range instances {
					instWg.Add(1)
					go func(inst TargetMatrixInstance) {
						defer instWg.Done()

						effectiveParamValues := targetExecutorEffectiveParameterValues(target, inv, inst.Bindings)
						effectiveInv := TargetInvocation{Parameters: effectiveParamValues}

						paramInstanceKey := TargetExecutorExecutionNodeInstanceKey(name)
						cacheInstanceKey := inst.CacheKey
						if cacheInstanceKey == "" && paramInstanceKey != "" {
							cacheInstanceKey = paramInstanceKey
						}

						depExecNodes := plan.Outgoing[canonicalName]
						if len(depExecNodes) != len(target.DependsOn) {
							depExecNodes = targetExecutorFallbackDependencyNodes(builtIR, target)
						}

						decision, cacheErr := targetExecutorEvaluateCache(
							builtIR,
							canonicalName,
							cacheInstanceKey,
							effectiveInv,
							depExecNodes,
							runOpts,
							depStateFingerprints,
							depOutputFingerprints,
						)
						if cacheErr != nil {
							instMu.Lock()
							if targetErr == nil {
								targetErr = cacheErr
							}
							instMu.Unlock()
							return
						}

						var outputFingerprint uint64
						if decision.Skip {
							targetExecutorEmitCacheSkipSignals(
								runOpts.SignalContext,
								canonicalName,
								cacheInstanceKey,
								decision.StateFingerprint,
							)
							outputFingerprint = decision.OutputFingerprint
						} else {
							runErr := TargetExecutorRunTarget(
								builtIR.SourceDirectory,
								target,
								builtIR.GlobalVariables,
								effectiveInv,
								runOpts,
							)
							if runErr != nil {
								instMu.Lock()
								if targetErr == nil {
									targetErr = runErr
								}
								instMu.Unlock()
								return
							}

							var commitErr error
							outputFingerprint, commitErr = targetExecutorCommitCache(
								builtIR,
								canonicalName,
								cacheInstanceKey,
								effectiveInv,
								depExecNodes,
								runOpts,
								depStateFingerprints,
								depOutputFingerprints,
							)
							if commitErr != nil {
								instMu.Lock()
								if targetErr == nil {
									targetErr = commitErr
								}
								instMu.Unlock()
								return
							}

							targetForCache := builtIR.Targets[canonicalName]
							if targetForCache.Artifacts != nil &&
								!targetForCache.Artifacts.Volatile &&
								runOpts.CacheStore != nil {
								targetExecutorEmitCacheUpdatedSignals(
									runOpts.SignalContext,
									canonicalName,
									decision.StateFingerprint,
									outputFingerprint,
								)
							}
						}

						instMu.Lock()
						if decision.StateFingerprint != 0 {
							instStateFingerprints = append(instStateFingerprints, decision.StateFingerprint)
						}
						if outputFingerprint != 0 {
							instOutputFingerprints = append(instOutputFingerprints, outputFingerprint)
						}
						instMu.Unlock()
					}(instance)
				}

				instWg.Wait()

				mu.Lock()
				defer mu.Unlock()
				if targetErr != nil {
					results[name] = targetErr
					return
				}
				if phaseErr != nil {
					return
				}

				results[name] = nil
				depStateFingerprints[name] = cache.CacheAggregateInstanceFingerprints(instStateFingerprints)
				depOutputFingerprints[name] = cache.CacheAggregateInstanceFingerprints(instOutputFingerprints)
			}(targetName)
		}

		wg.Wait()

		if phaseErr != nil {
			return phaseErr
		}
	}

	entryNode := canonicalEntry
	if err := results[entryNode]; err != nil {
		return err
	}

	return nil
}

func targetExecutorInvocationForNode(
	plan *TargetExecutionPlan,
	nodeID string,
	callerInvocations map[string]TargetInvocation,
) TargetInvocation {
	if inv, ok := plan.NodeInvocations[nodeID]; ok {
		return inv
	}

	canonical := TargetExecutorExecutionNodeCanonical(nodeID)
	if callerInvocations != nil {
		if inv, ok := callerInvocations[canonical]; ok {
			return inv
		}
	}

	return TargetInvocation{}
}

func targetExecutorClosureNames(chain [][]string) map[string]struct{} {
	closure := make(map[string]struct{})
	for _, phase := range chain {
		for _, name := range phase {
			closure[name] = struct{}{}
		}
	}
	return closure
}

func targetExecutorClosureRequiresConfirm(builtIR ir.HelmIR, closure map[string]struct{}) bool {
	for name := range closure {
		target := builtIR.Targets[name]
		for _, dep := range target.DependsOn {
			if dep.Confirm {
				return true
			}
		}
	}
	return false
}

func targetExecutorConfirmBeforeTarget(
	builtIR ir.HelmIR,
	dependencyName string,
	closure map[string]struct{},
	opts TargetExecutorOptions,
) error {
	for dependentName := range closure {
		dependent := builtIR.Targets[dependentName]
		for _, dep := range dependent.DependsOn {
			canonical, ok := ir.IRResolveTargetName(builtIR.Targets, dep.TargetName)
			if !ok || canonical != dependencyName || !dep.Confirm {
				continue
			}

			proceed, err := opts.ConfirmDependency(dependentName, dep)
			if err != nil {
				return err
			}
			if !proceed {
				return fmt.Errorf("execution aborted: user declined dependency '%s' required by '%s'", canonical, dependentName)
			}
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
	canonicalName := TargetExecutorExecutionNodeCanonical(executionNodeID)
	target := builtIR.Targets[canonicalName]

	depNodes := plan.Outgoing[canonicalName]
	if len(depNodes) != len(target.DependsOn) {
		depNodes = targetExecutorFallbackDependencyNodes(builtIR, target)
	}

	for i, dep := range target.DependsOn {
		depNode := dep.TargetName
		if i < len(depNodes) {
			depNode = depNodes[i]
		}

		depErr := results[depNode]
		if depErr == nil {
			continue
		}

		if dep.Optional {
			continue
		}

		depCanonical := TargetExecutorExecutionNodeCanonical(depNode)
		return TargetExecutorFormatDependencyBlockedError(canonicalName, depCanonical, depErr)
	}

	return nil
}

func targetExecutorFallbackDependencyNodes(
	builtIR ir.HelmIR,
	target ir.HelmTarget,
) []string {
	out := make([]string, len(target.DependsOn))
	for i, dep := range target.DependsOn {
		canonical, ok := ir.IRResolveTargetName(builtIR.Targets, dep.TargetName)
		if ok {
			out[i] = canonical
		} else {
			out[i] = dep.TargetName
		}
	}
	return out
}

func targetExecutorValidatePhaseTTY(phase []string, targets map[string]ir.HelmTarget) error {
	var interactiveTargets []string

	for _, targetName := range phase {
		target, ok := targets[targetName]
		if !ok {
			continue
		}
		if target.Interactive {
			interactiveTargets = append(interactiveTargets, targetName)
		}
	}

	if len(interactiveTargets) == 0 {
		return nil
	}

	if len(phase) > 1 {
		return fmt.Errorf(
			"%s: interactive target %q cannot run in parallel with %d other target(s) in the same phase",
			ERROR_TTY_CONFLICT,
			interactiveTargets[0],
			len(phase)-1,
		)
	}

	return nil
}
