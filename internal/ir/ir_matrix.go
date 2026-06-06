package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"syntaxa"
)

func handleTargetMatrix(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	currentTarget *HelmTarget,
	state *targetParseState,
) {
	if state.matrixDeclared {
		emitSemanticError(
			builder,
			node,
			ERROR_DUPLICATE_MATRIX,
			fmt.Sprintf("matrix block for target '%s' already declared", currentTarget.Name),
		)
		return
	}
	state.matrixDeclared = true

	if currentTarget.Interactive {
		emitSemanticError(
			builder,
			node,
			ERROR_INTERACTIVE_MATRIX,
			fmt.Sprintf("target '%s' cannot use interactive = true with a matrix block", currentTarget.Name),
		)
		return
	}

	identifierNode := node.FindDirectChildKind(artifacts.NodeMatrixIdentifier)
	if identifierNode == nil {
		emitSemanticError(builder, node, ERROR_EMPTY_MATRIX, "matrix block is missing a variable name")
		return
	}

	variableName := extractContentFromSingleTokenNode(builder, identifierNode)
	for _, param := range currentTarget.Parameters {
		if param.Name == variableName {
			emitSemanticError(
				builder,
				identifierNode,
				ERROR_MATRIX_PARAM_SHADOW,
				fmt.Sprintf("matrix variable '%s' cannot share a name with target parameter '%s'", variableName, param.Name),
			)
			return
		}
	}

	scope := resolveScopeForTarget(builder.effectiveGlobals(), currentTarget.Parameters)
	values := extractMatrixValues(builder, node, scope)
	if len(values) == 0 {
		emitSemanticError(
			builder,
			node,
			ERROR_EMPTY_MATRIX,
			fmt.Sprintf("matrix for target '%s' must have at least one value", currentTarget.Name),
		)
		return
	}

	currentTarget.Matrix = &HelmMatrix{
		VariableName: variableName,
		Values:       values,
	}
}

func extractMatrixValues(
	builder *irBuilder,
	matrixNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) []HelmMatrixValue {
	if arrayNode := matrixNode.FindFirstKind(artifacts.NodeMatrixArray); arrayNode != nil {
		return extractMatrixValuesFromArray(builder, arrayNode, scope)
	}

	for _, child := range matrixNode.ChildrenUnsafe() {
		if child.Kind() == artifacts.NodeMatrixIdentifier {
			continue
		}
		if varNode := artifactItemVarRefNode(child); varNode != nil {
			varName := extractContentFromSingleTokenNode(builder, varNode)
			if values, ok := matrixValuesFromGlobalArtifactItems(builder, varNode, scope, varName); ok {
				return values
			}
		}
		if value, ok := extractMatrixValueFromNode(builder, child, scope); ok {
			return []HelmMatrixValue{value}
		}
	}
	return nil
}

func extractMatrixValuesFromArray(
	builder *irBuilder,
	arrayNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) []HelmMatrixValue {
	var values []HelmMatrixValue
	for _, child := range arrayNode.ChildrenUnsafe() {
		if value, ok := extractMatrixValueFromNode(builder, child, scope); ok {
			values = append(values, value)
		}
	}
	return values
}

func extractMatrixValueFromNode(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) (HelmMatrixValue, bool) {
	if globNode := artifactItemGlobNode(node); globNode != nil {
		globIR, ok := evaluateGlob(builder, globNode, scope)
		if !ok {
			return HelmMatrixValue{}, false
		}
		return HelmMatrixValue{Kind: MatrixValueGlob, Glob: globIR}, true
	}

	if pathNode := artifactItemPathNode(node); pathNode != nil {
		return HelmMatrixValue{
			Kind:    MatrixValueLiteral,
			Literal: evaluatePath(builder, pathNode, scope),
		}, true
	}

	if strNode := artifactItemStringNode(node); strNode != nil {
		return HelmMatrixValue{
			Kind:    MatrixValueLiteral,
			Literal: extractStringFromStringNode(builder, strNode, scope),
		}, true
	}

	if varNode := artifactItemVarRefNode(node); varNode != nil {
		varName := extractContentFromSingleTokenNode(builder, varNode)
		if resolveScopeHasParameter(scope, varName) {
			return HelmMatrixValue{
				Kind:          MatrixValueParameterRef,
				ParameterName: varName,
			}, true
		}
		if text, ok := resolveScopeVariableAsLiteral(scope, varName); ok {
			return HelmMatrixValue{Kind: MatrixValueLiteral, Literal: text}, true
		}
		if globalVariableIsArtifactArray(scope, varName) {
			emitVariableNotScalar(builder, varNode, varName, "matrix value")
			return HelmMatrixValue{}, false
		}
		emitSemanticError(
			builder,
			varNode,
			ERROR_UNDECLARED_VARIABLE,
			fmt.Sprintf("use of undeclared variable '%s' in matrix", varName),
		)
		return HelmMatrixValue{}, false
	}

	return HelmMatrixValue{}, false
}

func matrixValuesFromGlobalArtifactItems(
	builder *irBuilder,
	varNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	varName string,
) ([]HelmMatrixValue, bool) {
	items, ok := resolveGlobalArtifactItems(scope, varName)
	if !ok {
		return nil, false
	}

	values := make([]HelmMatrixValue, 0, len(items))
	for _, item := range items {
		switch item.Kind {
		case ArtifactInputString:
			values = append(values, HelmMatrixValue{
				Kind:    MatrixValueLiteral,
				Literal: item.Literal,
			})
		case ArtifactInputGlob:
			if item.Glob == nil {
				emitSemanticError(
					builder,
					varNode,
					ERROR_INVALID_GLOB,
					fmt.Sprintf("variable '%s' contains an invalid glob entry for matrix", varName),
				)
				return nil, true
			}
			globCopy := *item.Glob
			values = append(values, HelmMatrixValue{Kind: MatrixValueGlob, Glob: &globCopy})
		default:
			emitSemanticError(
				builder,
				varNode,
				ERROR_INVALID_VARIABLE_VALUE,
				fmt.Sprintf("variable '%s' contains an unsupported matrix entry", varName),
			)
			return nil, true
		}
	}
	return values, true
}
