package targetexecutor

import (
	"fmt"
	"helm/internal/ir"
)

func targetExecutorEffectiveDependsOn(
	builtIR ir.HelmIR,
	target ir.HelmTarget,
	inv TargetInvocation,
	parentResolved TargetResolvedParameters,
) ([]ir.HelmTargetDependency, error) {
	paramValues := TargetExecutorParametersForTarget(target, inv)
	out := make([]ir.HelmTargetDependency, 0, len(target.DependsOn)+len(target.DependsOnParamNames))

	out = append(out, target.DependsOn...)

	for _, paramName := range target.DependsOnParamNames {
		value, ok := paramValues[paramName]
		if !ok {
			return nil, fmt.Errorf(
				"target '%s': depends_on parameter '%s' is not bound",
				target.Name,
				paramName,
			)
		}
		if value.Kind != ir.HelmParameterDependencyList {
			return nil, fmt.Errorf(
				"target '%s': parameter '%s' must be a dependency array",
				target.Name,
				paramName,
			)
		}
		out = append(out, value.Dependencies...)
	}

	if len(out) == 0 {
		return out, nil
	}

	bound := make([]ir.HelmTargetDependency, 0, len(out))
	for _, dep := range out {
		edge := dep
		if len(edge.Parameters) == 0 {
			bound = append(bound, edge)
			continue
		}

		edgeParams, bindErr := targetExecutorBindDependencyParams(
			builtIR.GlobalVariables,
			parentResolved,
			paramValues,
			edge.Parameters,
		)
		if bindErr != nil {
			return nil, fmt.Errorf(
				"target '%s' dependency '%s': %w",
				target.Name,
				edge.TargetName,
				bindErr,
			)
		}
		edge.Parameters = edgeParams
		bound = append(bound, edge)
	}

	return bound, nil
}

func targetExecutorDependencyNodesFromEdges(
	builtIR ir.HelmIR,
	dependentCanonical string,
	edges []ir.HelmTargetDependency,
) ([]string, error) {
	out := make([]string, len(edges))
	for i, dep := range edges {
		depCanonical, ok := ir.IRResolveTargetName(builtIR.Targets, dep.TargetName)
		if !ok {
			return nil, fmt.Errorf(
				"target '%s' depends on undeclared target '%s'",
				dependentCanonical,
				dep.TargetName,
			)
		}

		if len(dep.Parameters) == 0 {
			out[i] = depCanonical
			continue
		}

		out[i] = targetExecutorParametricInstanceID(depCanonical, dep.Parameters)
	}

	return out, nil
}

func targetExecutorInvocationForInsert(
	dependentName string,
	callerInvocations map[string]TargetInvocation,
	nodeInvocations map[string]TargetInvocation,
) TargetInvocation {
	if inv, ok := nodeInvocations[dependentName]; ok {
		return inv
	}
	if callerInvocations != nil {
		if inv, ok := callerInvocations[dependentName]; ok {
			return inv
		}
	}
	return TargetInvocation{}
}

func targetExecutorRegisterDependencyNodes(
	builtIR ir.HelmIR,
	dependentCanonical string,
	effectiveDeps []ir.HelmTargetDependency,
	nodeInvocations map[string]TargetInvocation,
) ([]string, error) {
	depNodes := make([]string, 0, len(effectiveDeps))
	for _, dep := range effectiveDeps {
		depCanonical, ok := ir.IRResolveTargetName(builtIR.Targets, dep.TargetName)
		if !ok {
			return nil, fmt.Errorf(
				"target '%s' depends on undeclared target '%s'",
				dependentCanonical,
				dep.TargetName,
			)
		}

		if len(dep.Parameters) == 0 {
			depNodes = append(depNodes, depCanonical)
			continue
		}

		execNode := targetExecutorParametricInstanceID(depCanonical, dep.Parameters)
		if existing, exists := nodeInvocations[execNode]; exists {
			if targetExecutorParameterMapFingerprint(existing.Parameters) !=
				targetExecutorParameterMapFingerprint(dep.Parameters) {
				return nil, fmt.Errorf("parametric instance collision for '%s'", depCanonical)
			}
		} else {
			nodeInvocations[execNode] = TargetInvocation{Parameters: dep.Parameters}
		}
		depNodes = append(depNodes, execNode)
	}
	return depNodes, nil
}
