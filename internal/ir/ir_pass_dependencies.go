package ir

import "fmt"

func validateTargetDependencies(builder *irBuilder) {
	for _, target := range builder.targets {
		seenDeps := make(map[string]struct{}, len(target.DependsOn))

		for _, dep := range target.DependsOn {
			canonical, exists := IRResolveTargetName(builder.targets, dep.TargetName)
			if !exists {
				emitSemanticError(
					builder,
					dep.SourceNode,
					ERROR_UNDECLARED_TARGET,
					fmt.Sprintf("dependency '%s' refers to undeclared target", dep.TargetName),
				)
				continue
			}

			if _, duplicate := seenDeps[canonical]; duplicate {
				emitSemanticError(
					builder,
					dep.SourceNode,
					ERROR_DUPLICATE_DEPENDENCY,
					fmt.Sprintf("target '%s' lists dependency '%s' more than once", target.Name, canonical),
				)
				continue
			}
			seenDeps[canonical] = struct{}{}

			validateDependencyParameters(builder, target.Name, dep, canonical)
		}
	}
}

func validateDependencyParameters(
	builder *irBuilder,
	dependentName string,
	dep HelmTargetDependency,
	dependencyCanonical string,
) {
	dependencyTarget := builder.targets[dependencyCanonical]

	declared := make(map[string]struct{}, len(dependencyTarget.Parameters))
	for _, param := range dependencyTarget.Parameters {
		declared[param.Name] = struct{}{}
	}

	for key, value := range dep.Parameters {
		if value.Kind == HelmParameterDependencyList {
			irMarkTargetParameterDependencyList(builder.targets, dependencyCanonical, key)
		}
		if _, ok := declared[key]; !ok {
			emitSemanticError(
				builder,
				dep.SourceNode,
				ERROR_UNKNOWN_DEPENDENCY_PARAM,
				fmt.Sprintf(
					"target '%s' passes unknown parameter '%s' to dependency '%s'",
					dependentName,
					key,
					dep.TargetName,
				),
			)
		}
	}

	for _, param := range dependencyTarget.Parameters {
		if param.Optional {
			continue
		}
		if _, ok := dep.Parameters[param.Name]; !ok {
			emitSemanticError(
				builder,
				dep.SourceNode,
				ERROR_MISSING_DEPENDENCY_PARAM,
				fmt.Sprintf(
					"target '%s' must pass required parameter '%s' to dependency '%s'",
					dependentName,
					param.Name,
					dep.TargetName,
				),
			)
			continue
		}
		value := dep.Parameters[param.Name]
		if param.DependencyList && value.Kind != HelmParameterDependencyList {
			emitSemanticError(
				builder,
				dep.SourceNode,
				ERROR_INVALID_VARIABLE_VALUE,
				fmt.Sprintf(
					"parameter '%s' for dependency '%s' must be a target dependency array",
					param.Name,
					dep.TargetName,
				),
			)
		}
		if !param.DependencyList && value.Kind == HelmParameterDependencyList {
			emitSemanticError(
				builder,
				dep.SourceNode,
				ERROR_INVALID_VARIABLE_VALUE,
				fmt.Sprintf(
					"parameter '%s' for dependency '%s' cannot be a target dependency array",
					param.Name,
					dep.TargetName,
				),
			)
		}
	}
}
