package targetexecutor

import (
	"fmt"
	"helm/internal/ir"
)

func TargetExecutorExecutionChain(builtIR ir.HelmIR, entryTarget string) ([][]string, error) {
	plan, err := TargetExecutorBuildExecutionPlan(builtIR, entryTarget, nil)
	if err != nil {
		return nil, err
	}
	return plan.Phases, nil
}

func targetExecutorInsertDependencies(
	builtIR ir.HelmIR,
	targetName string,
	graph map[string][]string,
) error {
	if _, alreadyProcessed := graph[targetName]; alreadyProcessed {
		return nil
	}

	deps, err := targetExecutorDependenciesForTarget(builtIR, targetName)
	if err != nil {
		return err
	}

	graph[targetName] = deps

	for _, dependencyName := range deps {
		if err := targetExecutorInsertDependencies(builtIR, dependencyName, graph); err != nil {
			return err
		}
	}

	return nil
}

func targetExecutorDependenciesForTarget(builtIR ir.HelmIR, targetName string) ([]string, error) {
	target, ok := builtIR.Targets[targetName]
	if !ok {
		return nil, fmt.Errorf("target '%s' does not exist in IR", targetName)
	}

	out := make([]string, len(target.DependsOn))
	for i, dependency := range target.DependsOn {
		canonical, ok := ir.IRResolveTargetName(builtIR.Targets, dependency.TargetName)
		if !ok {
			return nil, fmt.Errorf("target '%s' depends on undeclared target '%s'", targetName, dependency.TargetName)
		}
		out[i] = canonical
	}

	return out, nil
}
