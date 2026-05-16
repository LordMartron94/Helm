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
// Composite forms (path, glob) must be dispatched before leaf forms: FindFirstKind on the subtree would
// otherwise match string literals or variable references nested inside path().
func resolveArtifactItem(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) string {
	if pathNode := artifactItemPathNode(node); pathNode != nil {
		return evaluatePath(builder, pathNode, globalVariables)
	}

	if globNode := artifactItemGlobNode(node); globNode != nil {
		return "[EVALUATED_GLOB]"
	}

	if strNode := artifactItemStringNode(node); strNode != nil {
		scope := resolveScopeForGlobals(globalVariables)
		return extractStringFromStringNode(builder, strNode, scope)
	}

	if varNode := artifactItemVarRefNode(node); varNode != nil {
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

	return ""
}

func artifactItemPathNode(node *syntaxa.SyntaxaLSTNode[artifacts.Node]) *syntaxa.SyntaxaLSTNode[artifacts.Node] {
	if node.Kind() == artifacts.NodePath {
		return node
	}
	return node.FindDirectChildKind(artifacts.NodePath)
}

func artifactItemGlobNode(node *syntaxa.SyntaxaLSTNode[artifacts.Node]) *syntaxa.SyntaxaLSTNode[artifacts.Node] {
	if node.Kind() == artifacts.NodeGlob {
		return node
	}
	return node.FindDirectChildKind(artifacts.NodeGlob)
}

func artifactItemStringNode(node *syntaxa.SyntaxaLSTNode[artifacts.Node]) *syntaxa.SyntaxaLSTNode[artifacts.Node] {
	if node.Kind() == artifacts.NodeStringLiteral {
		return node
	}
	return node.FindDirectChildKind(artifacts.NodeStringLiteral)
}

func artifactItemVarRefNode(node *syntaxa.SyntaxaLSTNode[artifacts.Node]) *syntaxa.SyntaxaLSTNode[artifacts.Node] {
	if node.Kind() == artifacts.NodeVariableReference {
		return node
	}
	return node.FindDirectChildKind(artifacts.NodeVariableReference)
}

func extractBooleanNode(builder *irBuilder, node *syntaxa.SyntaxaLSTNode[artifacts.Node]) bool {
	if node == nil {
		return false
	}
	val := extractContentFromSingleTokenNode(builder, node)
	return val == "true"
}
