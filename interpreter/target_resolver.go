package interpreter

import (
	"fmt"
	"helm/internal/ir"
	"structarch"
)

func executionChainForTarget(builtIR ir.HelmIR, targetName string) ([][]string, error) {
	if target, ok := builtIR.Targets[targetName]; !ok {
		return nil, fmt.Errorf("target '%s' does not exist in IR", targetName)
	} else {
		graph := map[string][]string{}
		if err := insertTargetDependenciesInGraph(builtIR, target.Name, graph); err != nil {
			return nil, err
		}

		chain, err := structarch.STRUCTARCH_DAG_Resolve(graph)

		if err != nil {
			return nil, err
		}

		return chain, nil
	}
}

func insertTargetDependenciesInGraph(builtIR ir.HelmIR, targetName string, graph map[string][]string) error {
	if _, alreadyProcessed := graph[targetName]; alreadyProcessed {
		return nil
	}

	targetDeps, err := dependenciesForTarget(builtIR, targetName)

	if err != nil {
		return err
	}

	graph[targetName] = targetDeps

	for _, dependencyName := range targetDeps {
		insertTargetDependenciesInGraph(builtIR, dependencyName, graph)
	}

	return nil
}

func dependenciesForTarget(builtIR ir.HelmIR, targetName string) ([]string, error) {
	if target, ok := builtIR.Targets[targetName]; !ok {
		return nil, fmt.Errorf("target '%s' does not exist in IR", targetName)
	} else {
		out := make([]string, len(target.DependsOn))
		for i, dependency := range target.DependsOn {
			out[i] = dependency.TargetName
		}

		return out, nil
	}
}
