package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"syntaxa"
)

const (
	HelmGlobalVarString HelmGlobalVariableKind = iota
	HelmGlobalVarArtifactArray
	HelmGlobalVarStringList
)

type HelmGlobalVariableKind int

type HelmGlobalVariable struct {
	Kind          HelmGlobalVariableKind
	StringValue   string
	ArtifactItems []HelmArtifactInput
	StringList    []string
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

func helmGlobalVariableCopy(variable HelmGlobalVariable) HelmGlobalVariable {
	switch variable.Kind {
	case HelmGlobalVarArtifactArray:
		items := make([]HelmArtifactInput, len(variable.ArtifactItems))
		copy(items, variable.ArtifactItems)
		return HelmGlobalVariable{
			Kind:          HelmGlobalVarArtifactArray,
			ArtifactItems: items,
		}
	case HelmGlobalVarStringList:
		list := append([]string(nil), variable.StringList...)
		return HelmGlobalVariable{
			Kind:       HelmGlobalVarStringList,
			StringList: list,
		}
	default:
		return HelmGlobalVariable{
			Kind:        HelmGlobalVarString,
			StringValue: variable.StringValue,
		}
	}
}

func resolveGlobalValueFromValueNode(
	builder *irBuilder,
	valueNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	variableLabel string,
) (HelmGlobalVariable, bool) {
	arrayNode := valueNode.FindFirstKind(artifacts.NodeVariableArray)
	if arrayNode != nil {
		items := extractArtifactItemsFromPathArrayRoot(builder, arrayNode, scope)
		if len(items) == 0 {
			emitSemanticError(
				builder,
				valueNode,
				ERROR_INVALID_VARIABLE_VALUE,
				fmt.Sprintf("%s array must contain at least one entry", variableLabel),
			)
			return HelmGlobalVariable{}, false
		}
		return HelmGlobalVariable{
			Kind:          HelmGlobalVarArtifactArray,
			ArtifactItems: items,
		}, true
	}

	if varRefNode := valueNode.FindFirstKind(artifacts.NodeVariableReference); varRefNode != nil {
		refName := extractContentFromSingleTokenNode(builder, varRefNode)
		variable, ok := scope.globals[refName]
		if !ok {
			emitSemanticError(
				builder,
				varRefNode,
				ERROR_UNDECLARED_VARIABLE,
				fmt.Sprintf("%s references undeclared variable '%s'", variableLabel, refName),
			)
			return HelmGlobalVariable{}, false
		}
		return helmGlobalVariableCopy(variable), true
	}

	stringNode := findStringContentNode(valueNode)
	if stringNode == nil {
		emitSemanticError(
			builder,
			valueNode,
			ERROR_INVALID_VARIABLE_VALUE,
			fmt.Sprintf("%s must be a string literal, variable reference, or artifact array", variableLabel),
		)
		return HelmGlobalVariable{}, false
	}

	return HelmGlobalVariable{
		Kind:        HelmGlobalVarString,
		StringValue: extractStringFromStringNode(builder, stringNode, scope),
	}, true
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
