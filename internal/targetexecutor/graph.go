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
	var resultsMu sync.RWMutex
	var fingerprintMu sync.RWMutex

	runOpts := opts
	if runOpts.Targets == nil {
		runOpts.Targets = builtIR.Targets
	}
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

	for phaseIndex, phase := range plan.Phases {
		if err := targetExecutorValidatePhaseTTY(phase, builtIR.Targets); err != nil {
			return err
		}

		if runOpts.RunTranscript != nil {
			runOpts.RunTranscript.PhaseStart(phaseIndex + 1)
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

				canonicalName := TargetExecutorExecutionNodeCanonical(name)
				target := builtIR.Targets[canonicalName]
				inv := targetExecutorInvocationForNode(plan, name, invocations)

				var blockedErr error
				resultsMu.RLock()
				blockedErr = targetExecutorDependencyBlocked(plan, builtIR, name, inv, results)
				resultsMu.RUnlock()
				if blockedErr != nil {
					resultsMu.Lock()
					results[name] = blockedErr
					resultsMu.Unlock()
					return
				}

				paramValues := TargetExecutorParametersForTarget(target, inv)
				resolved, resolveErr := TargetExecutorResolveInvocationParameters(
					builtIR.SourceDirectory,
					builtIR.GlobalVariables,
					paramValues,
				)
				if resolveErr != nil {
					resultsMu.Lock()
					results[name] = resolveErr
					resultsMu.Unlock()
					return
				}
				baseInterpCtx, globalErr := TargetExecutorInterpolationGlobals(
					builtIR.SourceDirectory,
					builtIR.GlobalVariables,
					resolved,
				)
				if globalErr != nil {
					resultsMu.Lock()
					results[name] = globalErr
					resultsMu.Unlock()
					return
				}
				interpCtx, globalErr := TargetExecutorEvaluateLetBindings(
					builtIR.SourceDirectory,
					target,
					baseInterpCtx,
				)
				if globalErr != nil {
					resultsMu.Lock()
					results[name] = globalErr
					resultsMu.Unlock()
					return
				}

				instances, instanceErr := TargetExecutorMatrixInstances(
					builtIR.SourceDirectory,
					target,
					paramValues,
					builtIR.GlobalVariables,
					interpCtx,
				)
				if instanceErr != nil {
					resultsMu.Lock()
					results[name] = instanceErr
					resultsMu.Unlock()
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

						depExecNodes, depNodesErr := targetExecutorDependencyNodes(
							plan,
							builtIR,
							name,
							inv,
						)
						if depNodesErr != nil {
							instMu.Lock()
							if targetErr == nil {
								targetErr = depNodesErr
							}
							instMu.Unlock()
							return
						}

						fingerprintMu.RLock()
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
						fingerprintMu.RUnlock()
						if cacheErr != nil {
							instMu.Lock()
							if targetErr == nil {
								targetErr = cacheErr
							}
							instMu.Unlock()
							return
						}

						transcriptNode := targetExecutorTranscriptNodeID(name, inst.CacheKey)
						if runOpts.RunTranscript != nil {
							runOpts.RunTranscript.TargetStart(transcriptNode, target.Interactive)
						}

						var outputFingerprint uint64
						var instanceRunErr error
						if decision.Skip {
							targetExecutorEmitCacheSkipSignals(
								runOpts.SignalContext,
								canonicalName,
								cacheInstanceKey,
								decision.StateFingerprint,
							)
							outputFingerprint = decision.OutputFingerprint
						} else {
							instanceRunOpts := runOpts
							instanceRunOpts.TranscriptNodeID = transcriptNode
							if canonicalName == canonicalEntry {
								targetExecutorMarkEntryReached(runOpts.RunState)
							}

							instanceRunErr = TargetExecutorRunTarget(
								builtIR.SourceDirectory,
								target,
								builtIR.GlobalVariables,
								effectiveInv,
								instanceRunOpts,
							)
							if instanceRunErr != nil {
								instMu.Lock()
								if targetErr == nil {
									targetErr = instanceRunErr
								}
								instMu.Unlock()
							}
						}

						if runOpts.RunTranscript != nil {
							runOpts.RunTranscript.TargetEnd(transcriptNode, instanceRunErr)
						}

						if instanceRunErr != nil {
							return
						}

						if !decision.Skip {

							var commitErr error
							fingerprintMu.RLock()
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
							fingerprintMu.RUnlock()
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

				if targetErr != nil {
					resultsMu.Lock()
					results[name] = targetErr
					resultsMu.Unlock()
					return
				}
				mu.Lock()
				if phaseErr != nil {
					mu.Unlock()
					return
				}
				mu.Unlock()

				fingerprintMu.Lock()
				depStateFingerprints[name] = cache.CacheAggregateInstanceFingerprints(instStateFingerprints)
				depOutputFingerprints[name] = cache.CacheAggregateInstanceFingerprints(instOutputFingerprints)
				fingerprintMu.Unlock()
				resultsMu.Lock()
				results[name] = nil
				resultsMu.Unlock()
			}(targetName)
		}

		wg.Wait()

		if phaseErr != nil {
			return phaseErr
		}

		if runOpts.RunTranscript != nil {
			runOpts.RunTranscript.PhaseEnd(phaseIndex + 1)
		}
	}

	entryNode := canonicalEntry
	resultsMu.RLock()
	entryErr := results[entryNode]
	resultsMu.RUnlock()
	if entryErr != nil {
		return targetExecutorPreferEntryProcessExit(entryErr, entryNode)
	}

	return nil
}

func targetExecutorTranscriptNodeID(nodeID string, matrixCacheKey string) string {
	if matrixCacheKey == "" {
		return nodeID
	}
	return nodeID + "#matrix:" + matrixCacheKey
}

func targetExecutorMarkEntryReached(state *TargetExecutorRunState) {
	if state == nil {
		return
	}
	state.EntryReached = true
}

func targetExecutorPreferEntryProcessExit(err error, entryNode string) error {
	exitErr, ok := err.(TargetExecutorProcessExitError)
	if !ok {
		return err
	}
	exitErr.Target = entryNode
	return exitErr
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
		canonical := TargetExecutorExecutionNodeCanonical(name)
		target, ok := builtIR.Targets[canonical]
		if !ok {
			continue
		}
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
	for dependentNodeID := range closure {
		dependentName := TargetExecutorExecutionNodeCanonical(dependentNodeID)
		dependent, ok := builtIR.Targets[dependentName]
		if !ok {
			continue
		}
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

func targetExecutorValidatePhaseTTY(phase []string, targets map[string]ir.HelmTarget) error {
	var interactiveTargets []string

	for _, targetName := range phase {
		canonical := TargetExecutorExecutionNodeCanonical(targetName)
		target, ok := targets[canonical]
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
