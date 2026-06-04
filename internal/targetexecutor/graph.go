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
	chain, err := TargetExecutorExecutionChain(builtIR, entryTarget)
	if err != nil {
		return err
	}

	canonicalEntry, ok := ir.IRResolveTargetName(builtIR.Targets, entryTarget)
	if !ok {
		return fmt.Errorf("target '%s' does not exist in IR", entryTarget)
	}

	closure := targetExecutorClosureNames(chain)
	if targetExecutorClosureRequiresConfirm(builtIR, closure) && opts.ConfirmDependency == nil {
		return fmt.Errorf("%s: dependency confirmation required but ConfirmDependency callback is nil", ERROR_CONFIRM_CALLBACK_REQUIRED)
	}

	effectiveInvocations, err := targetExecutorResolveEffectiveInvocations(builtIR, closure, invocations)
	if err != nil {
		return err
	}

	results := make(map[string]error, len(closure))
	depStateFingerprints := make(map[string]uint64, len(closure))
	depOutputFingerprints := make(map[string]uint64, len(closure))

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

	for _, phase := range chain {
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

				if err := targetExecutorConfirmBeforeTarget(builtIR, name, closure, runOpts); err != nil {
					mu.Lock()
					if phaseErr == nil {
						phaseErr = err
					}
					mu.Unlock()
					return
				}

				if blockedErr := targetExecutorDependencyBlocked(builtIR, name, results); blockedErr != nil {
					mu.Lock()
					results[name] = blockedErr
					mu.Unlock()
					return
				}

				target := builtIR.Targets[name]
				inv := TargetInvocation{}
				if mapped, exists := effectiveInvocations[name]; exists {
					inv = mapped
				}

				instances, instanceErr := TargetExecutorMatrixInstances(
					builtIR.SourceDirectory,
					target,
					ir.InterpolationGlobalsFromHelmGlobals(builtIR.GlobalVariables),
					TargetExecutorParametersForTarget(target, inv),
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

						effectiveParams := TargetExecutorEffectiveParameters(target, inv, inst.Bindings)
						effectiveInv := TargetInvocation{Parameters: effectiveParams}

						decision, cacheErr := targetExecutorEvaluateCache(
							builtIR,
							name,
							inst.CacheKey,
							effectiveInv,
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
								name,
								inst.CacheKey,
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
								name,
								inst.CacheKey,
								effectiveInv,
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

							targetForCache := builtIR.Targets[name]
							if targetForCache.Artifacts != nil &&
								!targetForCache.Artifacts.Volatile &&
								runOpts.CacheStore != nil {
								targetExecutorEmitCacheUpdatedSignals(
									runOpts.SignalContext,
									name,
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

	if err := results[canonicalEntry]; err != nil {
		return err
	}

	return nil
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
	builtIR ir.HelmIR,
	targetName string,
	results map[string]error,
) error {
	target := builtIR.Targets[targetName]

	for _, dep := range target.DependsOn {
		canonical, ok := ir.IRResolveTargetName(builtIR.Targets, dep.TargetName)
		if !ok {
			continue
		}

		depErr := results[canonical]
		if depErr == nil {
			continue
		}

		if dep.Optional {
			continue
		}

		return TargetExecutorFormatDependencyBlockedError(targetName, canonical, depErr)
	}

	return nil
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
