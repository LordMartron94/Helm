package targetexecutor

import (
	"fmt"
	"helm/internal/ir"
)

func targetExecutorResolveEffectiveInvocations(
	builtIR ir.HelmIR,
	closure map[string]struct{},
	callerInvocations map[string]TargetInvocation,
) (map[string]TargetInvocation, error) {
	effective := make(map[string]TargetInvocation, len(closure))

	for dependencyName := range closure {
		edgeMaps, dependents, err := targetExecutorCollectDependencyParamMaps(
			builtIR,
			closure,
			dependencyName,
			callerInvocations,
		)
		if err != nil {
			return nil, err
		}

		var merged map[string]ir.HelmParameterValue
		if len(edgeMaps) > 0 {
			merged = edgeMaps[0]
			for i := 1; i < len(edgeMaps); i++ {
				if targetExecutorParameterMapFingerprint(edgeMaps[i]) != targetExecutorParameterMapFingerprint(merged) {
					return nil, fmt.Errorf(
						"conflicting parameters for dependency '%s' from '%s' and '%s'",
						dependencyName,
						dependents[0],
						dependents[i],
					)
				}
			}
		}

		if callerInvocations != nil {
			if caller, exists := callerInvocations[dependencyName]; exists {
				if merged == nil {
					merged = make(map[string]ir.HelmParameterValue)
				}
				for key, value := range TargetInvocationParameters(caller) {
					merged[key] = value
				}
			}
		}

		if len(merged) > 0 {
			effective[dependencyName] = TargetInvocation{Parameters: merged}
		}
	}

	return effective, nil
}

func targetExecutorCollectDependencyParamMaps(
	builtIR ir.HelmIR,
	closure map[string]struct{},
	dependencyName string,
	callerInvocations map[string]TargetInvocation,
) ([]map[string]ir.HelmParameterValue, []string, error) {
	var edgeMaps []map[string]ir.HelmParameterValue
	var dependents []string

	for dependentName := range closure {
		dependent, ok := builtIR.Targets[dependentName]
		if !ok {
			continue
		}

		parentResolved, err := targetExecutorResolvedParamsForTarget(
			builtIR,
			closure,
			dependentName,
			callerInvocations,
			nil,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("target '%s': %w", dependentName, err)
		}

		for _, dep := range dependent.DependsOn {
			canonical, ok := ir.IRResolveTargetName(builtIR.Targets, dep.TargetName)
			if !ok || canonical != dependencyName {
				continue
			}

			bound, err := targetExecutorBindDependencyParams(
				builtIR.GlobalVariables,
				parentResolved,
				dep.Parameters,
			)
			if err != nil {
				return nil, nil, fmt.Errorf("target '%s' dependency '%s': %w", dependentName, dep.TargetName, err)
			}

			edgeMaps = append(edgeMaps, bound)
			dependents = append(dependents, dependentName)
		}
	}

	return edgeMaps, dependents, nil
}

// targetExecutorResolvedParamsForTarget returns the effective string parameters for a target
// in the current closure: CLI invocation overrides, otherwise merged from dependents' depends_on edges.
func targetExecutorResolvedParamsForTarget(
	builtIR ir.HelmIR,
	closure map[string]struct{},
	targetName string,
	callerInvocations map[string]TargetInvocation,
	visiting map[string]struct{},
) (map[string]string, error) {
	if visiting == nil {
		visiting = make(map[string]struct{})
	}
	if _, onStack := visiting[targetName]; onStack {
		return nil, fmt.Errorf("cyclic parameter resolution involving target '%s'", targetName)
	}
	visiting[targetName] = struct{}{}
	defer delete(visiting, targetName)

	if callerInvocations != nil {
		if inv, exists := callerInvocations[targetName]; exists {
			target, ok := builtIR.Targets[targetName]
			if !ok {
				return nil, fmt.Errorf("target '%s' does not exist in IR", targetName)
			}
			paramValues := TargetExecutorParametersForTarget(target, inv)
			return TargetExecutorResolveInvocationParameters(
				builtIR.SourceDirectory,
				builtIR.GlobalVariables,
				paramValues,
			)
		}
	}

	var edgeMaps []map[string]ir.HelmParameterValue
	for dependentName := range closure {
		dependent, ok := builtIR.Targets[dependentName]
		if !ok {
			continue
		}

		for _, dep := range dependent.DependsOn {
			canonical, ok := ir.IRResolveTargetName(builtIR.Targets, dep.TargetName)
			if !ok || canonical != targetName || len(dep.Parameters) == 0 {
				continue
			}

			dependentResolved, err := targetExecutorResolvedParamsForTarget(
				builtIR,
				closure,
				dependentName,
				callerInvocations,
				visiting,
			)
			if err != nil {
				return nil, err
			}

			bound, err := targetExecutorBindDependencyParams(
				builtIR.GlobalVariables,
				dependentResolved,
				dep.Parameters,
			)
			if err != nil {
				return nil, fmt.Errorf("target '%s' dependency '%s': %w", dependentName, dep.TargetName, err)
			}
			edgeMaps = append(edgeMaps, bound)
		}
	}

	if len(edgeMaps) == 0 {
		return map[string]string{}, nil
	}

	merged := edgeMaps[0]
	for i := 1; i < len(edgeMaps); i++ {
		if targetExecutorParameterMapFingerprint(edgeMaps[i]) != targetExecutorParameterMapFingerprint(merged) {
			return nil, fmt.Errorf("conflicting parameters for target '%s' from multiple dependents", targetName)
		}
	}

	return TargetExecutorResolveInvocationParameters(
		builtIR.SourceDirectory,
		builtIR.GlobalVariables,
		merged,
	)
}
