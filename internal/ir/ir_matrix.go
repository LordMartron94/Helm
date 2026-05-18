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

	scope := resolveScopeForTarget(builder.globalVariables, currentTarget.Parameters)
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
		if text, ok := resolveInterpolation(scope, varName); ok {
			return HelmMatrixValue{Kind: MatrixValueLiteral, Literal: text}, true
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
