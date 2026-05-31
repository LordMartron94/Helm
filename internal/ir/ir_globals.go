package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"syntaxa"
)

const (
	HelmGlobalVarString HelmGlobalVariableKind = iota
	HelmGlobalVarStringArray
)

type HelmGlobalVariableKind int

type HelmGlobalVariable struct {
	Kind         HelmGlobalVariableKind
	StringValue  string
	StringValues []string
}

func InterpolationGlobalsFromHelmGlobals(globals map[string]HelmGlobalVariable) map[string]string {
	if len(globals) == 0 {
		return nil
	}
	out := make(map[string]string, len(globals))
	for name, variable := range globals {
		if variable.Kind == HelmGlobalVarString {
			out[name] = variable.StringValue
		}
	}
	return out
}

func resolveGlobalString(scope resolveScope, name string) (string, bool) {
	variable, ok := scope.globals[name]
	if !ok {
		return "", false
	}
	if variable.Kind != HelmGlobalVarString {
		return "", false
	}
	return variable.StringValue, true
}

func resolveGlobalStringList(scope resolveScope, name string) ([]string, bool) {
	variable, ok := scope.globals[name]
	if !ok {
		return nil, false
	}
	switch variable.Kind {
	case HelmGlobalVarString:
		return []string{variable.StringValue}, true
	case HelmGlobalVarStringArray:
		out := make([]string, len(variable.StringValues))
		copy(out, variable.StringValues)
		return out, true
	default:
		return nil, false
	}
}

func emitVariableNotScalar(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	name string,
	context string,
) {
	emitSemanticError(
		builder,
		node,
		ERROR_VARIABLE_ARRAY_NOT_SCALAR,
		fmt.Sprintf("variable '%s' is a string array and cannot be used as a single %s", name, context),
	)
}
