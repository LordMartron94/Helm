package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"syntaxa"
)

const (
	HelmGlobalVarString HelmGlobalVariableKind = iota
	HelmGlobalVarArtifactArray
)

type HelmGlobalVariableKind int

type HelmGlobalVariable struct {
	Kind          HelmGlobalVariableKind
	StringValue   string
	ArtifactItems []HelmArtifactInput
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

func resolveGlobalArtifactItems(scope resolveScope, name string) ([]HelmArtifactInput, bool) {
	variable, ok := scope.globals[name]
	if !ok {
		return nil, false
	}
	if variable.Kind != HelmGlobalVarArtifactArray {
		return nil, false
	}
	out := make([]HelmArtifactInput, len(variable.ArtifactItems))
	copy(out, variable.ArtifactItems)
	return out, true
}

func globalVariableIsArtifactArray(scope resolveScope, name string) bool {
	variable, ok := scope.globals[name]
	return ok && variable.Kind == HelmGlobalVarArtifactArray
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
		fmt.Sprintf("variable '%s' is an artifact array and cannot be used as a single %s", name, context),
	)
}
