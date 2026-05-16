package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"syntaxa"
)

func extractArtifactItemSequence(
	builder *irBuilder,
	parentNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) []string {
	var resolvedItems []string

	if arrayNode := parentNode.FindFirstKind(artifacts.NodeInputArray); arrayNode != nil {
		return extractItemsFromChildren(builder, arrayNode, globalVariables)
	}
	if arrayNode := parentNode.FindFirstKind(artifacts.NodeOutputArray); arrayNode != nil {
		return extractItemsFromChildren(builder, arrayNode, globalVariables)
	}

	singleItem := resolveArtifactItem(builder, parentNode, globalVariables)
	if singleItem != "" {
		resolvedItems = append(resolvedItems, singleItem)
	}

	return resolvedItems
}

func extractItemsFromChildren(
	builder *irBuilder,
	arrayNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) []string {
	var items []string
	for _, child := range arrayNode.ChildrenUnsafe() {
		resolved := resolveArtifactItem(builder, child, globalVariables)
		if resolved != "" {
			items = append(items, resolved)
		}
	}
	return items
}

// resolveArtifactItem resolves artifact paths; only global variables are supported (not target parameters).
func resolveArtifactItem(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) string {
	if strNode := node.FindFirstKind(artifacts.NodeStringLiteral); strNode != nil {
		scope := resolveScopeForGlobals(globalVariables)
		return extractStringFromStringNode(builder, strNode, scope)
	}

	if varNode := node.FindFirstKind(artifacts.NodeVariableReference); varNode != nil {
		varName := extractContentFromSingleTokenNode(builder, varNode)
		if val, exists := globalVariables[varName]; exists {
			return val
		}
		emitSemanticError(
			builder,
			varNode,
			ERROR_UNDECLARED_VARIABLE,
			fmt.Sprintf("use of undeclared variable '%s' in artifacts", varName),
		)
		return ""
	}

	if pathNode := node.FindFirstKind(artifacts.NodePath); pathNode != nil {
		return "[EVALUATED_PATH]"
	}
	if globNode := node.FindFirstKind(artifacts.NodeGlob); globNode != nil {
		return "[EVALUATED_GLOB]"
	}

	return ""
}

func extractBooleanNode(builder *irBuilder, node *syntaxa.SyntaxaLSTNode[artifacts.Node]) bool {
	if node == nil {
		return false
	}
	val := extractContentFromSingleTokenNode(builder, node)
	return val == "true"
}
