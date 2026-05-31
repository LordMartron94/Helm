package targetexecutor

import (
	"fmt"
	"helm/internal/ir"
	"sort"
	"strings"
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

		merged := map[string]string{}
		if len(edgeMaps) > 0 {
			merged = edgeMaps[0]
			for i := 1; i < len(edgeMaps); i++ {
				if targetExecutorParamMapFingerprint(edgeMaps[i]) != targetExecutorParamMapFingerprint(merged) {
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
) ([]map[string]string, []string, error) {
	var edgeMaps []map[string]string
	var dependents []string

	for dependentName := range closure {
		dependent, ok := builtIR.Targets[dependentName]
		if !ok {
			continue
		}

		parentParams := map[string]string{}
		if callerInvocations != nil {
			if inv, exists := callerInvocations[dependentName]; exists {
				parentParams = TargetInvocationParameters(inv)
			}
		}

		for _, dep := range dependent.DependsOn {
			canonical, ok := ir.IRResolveTargetName(builtIR.Targets, dep.TargetName)
			if !ok || canonical != dependencyName {
				continue
			}

			interpolated := targetExecutorInterpolateParamMap(
				ir.InterpolationGlobalsFromHelmGlobals(builtIR.GlobalVariables),
				parentParams,
				dep.Parameters,
			)

			edgeMaps = append(edgeMaps, interpolated)
			dependents = append(dependents, dependentName)
		}
	}

	return edgeMaps, dependents, nil
}

func targetExecutorInterpolateParamMap(
	globalVars map[string]string,
	parentParams map[string]string,
	raw map[string]string,
) map[string]string {
	if len(raw) == 0 {
		return map[string]string{}
	}

	out := make(map[string]string, len(raw))
	for key, value := range raw {
		out[key] = TargetExecutorInterpolateLiteral(value, globalVars, parentParams)
	}
	return out
}

func targetExecutorParamMapFingerprint(parameters map[string]string) string {
	if len(parameters) == 0 {
		return ""
	}

	keys := make([]string, 0, len(parameters))
	for key := range parameters {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var buffer strings.Builder
	for _, key := range keys {
		buffer.WriteString(key)
		buffer.WriteByte(0)
		buffer.WriteString(parameters[key])
		buffer.WriteByte(0)
	}
	return buffer.String()
}
