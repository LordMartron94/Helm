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

		parentResolved := map[string]string{}
		if callerInvocations != nil {
			if inv, exists := callerInvocations[dependentName]; exists {
				paramValues := TargetExecutorParametersForTarget(dependent, inv)
				resolved, err := TargetExecutorResolveInvocationParameters(
					builtIR.SourceDirectory,
					builtIR.GlobalVariables,
					paramValues,
				)
				if err != nil {
					return nil, nil, fmt.Errorf("target '%s': %w", dependentName, err)
				}
				parentResolved = resolved
			}
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
