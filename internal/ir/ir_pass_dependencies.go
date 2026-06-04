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

	dependentCanonical, _ := IRResolveTargetName(builder.targets, dependentName)

	for key, value := range dep.Parameters {
		if value.Kind == HelmParameterDependencyList {
			irMarkTargetParameterDependencyList(builder.targets, dependencyCanonical, key)
		}
		if value.Kind == HelmParameterTargetParamRef {
			if calleeParam, ok := irTargetParameterByName(dependencyTarget.Parameters, key); ok &&
				calleeParam.DependencyList &&
				dependencyParameterSatisfiesDependencyList(builder, dependentCanonical, key, value) {
				irMarkTargetParameterDependencyList(builder.targets, dependentCanonical, value.TargetParamName)
			}
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
		if param.DependencyList &&
			!dependencyParameterSatisfiesDependencyList(builder, dependentCanonical, param.Name, value) {
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

func irTargetParameterByName(parameters []HelmTargetParameter, name string) (HelmTargetParameter, bool) {
	for _, param := range parameters {
		if param.Name == name {
			return param, true
		}
	}
	return HelmTargetParameter{}, false
}

// dependencyParameterSatisfiesDependencyList accepts literal dependency arrays and
// caller parameter forwards (DEPENDENCIES = DEPENDENCIES) used by template wrappers.
func dependencyParameterSatisfiesDependencyList(
	builder *irBuilder,
	dependentCanonical string,
	dependencyParamName string,
	value HelmParameterValue,
) bool {
	switch value.Kind {
	case HelmParameterDependencyList:
		return true
	case HelmParameterTargetParamRef:
		if value.TargetParamName == dependencyParamName {
			return true
		}
		dependent, ok := builder.targets[dependentCanonical]
		if !ok {
			return false
		}
		for _, param := range dependent.Parameters {
			if param.Name == value.TargetParamName && param.DependencyList {
				return true
			}
		}
		return false
	default:
		return false
	}
}
