package targetexecutor

import (
	"fmt"
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

	results := make(map[string]error, len(closure))

	for _, phase := range chain {
		var wg sync.WaitGroup
		var mu sync.Mutex
		var phaseErr error

		for _, targetName := range phase {
			wg.Add(1)
			go func(name string) {
				defer wg.Done()

				if err := targetExecutorConfirmBeforeTarget(builtIR, name, closure, opts); err != nil {
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
				if invocations != nil {
					if mapped, exists := invocations[name]; exists {
						inv = mapped
					}
				}

				runErr := TargetExecutorRunTarget(target, builtIR.GlobalVariables, inv, opts)

				mu.Lock()
				results[name] = runErr
				mu.Unlock()
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
