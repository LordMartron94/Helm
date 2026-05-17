package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"syntaxa"
)

func extractInputArtifactSequence(
	builder *irBuilder,
	parentNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) []HelmArtifactInput {
	var resolvedItems []HelmArtifactInput

	if arrayNode := parentNode.FindFirstKind(artifacts.NodeInputArray); arrayNode != nil {
		return extractInputItemsFromChildren(builder, arrayNode, globalVariables)
	}

	item, ok := resolveInputArtifactItem(builder, parentNode, globalVariables)
	if ok {
		resolvedItems = append(resolvedItems, item)
	}

	return resolvedItems
}

func extractInputItemsFromChildren(
	builder *irBuilder,
	arrayNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) []HelmArtifactInput {
	var items []HelmArtifactInput
	for _, child := range arrayNode.ChildrenUnsafe() {
		item, ok := resolveInputArtifactItem(builder, child, globalVariables)
		if ok {
			items = append(items, item)
		}
	}
	return items
}

func resolveInputArtifactItem(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) (HelmArtifactInput, bool) {
	if globNode := artifactItemGlobNode(node); globNode != nil {
		globIR, ok := evaluateGlob(builder, globNode, globalVariables)
		if !ok {
			return HelmArtifactInput{}, false
		}
		return HelmArtifactInput{Kind: ArtifactInputGlob, Glob: globIR}, true
	}

	if pathNode := artifactItemPathNode(node); pathNode != nil {
		return HelmArtifactInput{
			Kind:    ArtifactInputString,
			Literal: evaluatePath(builder, pathNode, globalVariables),
		}, true
	}

	if strNode := artifactItemStringNode(node); strNode != nil {
		scope := resolveScopeForGlobals(globalVariables)
		return HelmArtifactInput{
			Kind:    ArtifactInputString,
			Literal: extractStringFromStringNode(builder, strNode, scope),
		}, true
	}

	if varNode := artifactItemVarRefNode(node); varNode != nil {
		varName := extractContentFromSingleTokenNode(builder, varNode)
		if val, exists := globalVariables[varName]; exists {
			return HelmArtifactInput{Kind: ArtifactInputString, Literal: val}, true
		}
		emitSemanticError(
			builder,
			varNode,
			ERROR_UNDECLARED_VARIABLE,
			fmt.Sprintf("use of undeclared variable '%s' in artifacts", varName),
		)
		return HelmArtifactInput{}, false
	}

	return HelmArtifactInput{}, false
}

func extractOutputArtifactSequence(
	builder *irBuilder,
	parentNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) []string {
	var resolvedItems []string

	if arrayNode := parentNode.FindFirstKind(artifacts.NodeOutputArray); arrayNode != nil {
		return extractOutputItemsFromChildren(builder, arrayNode, globalVariables)
	}

	singleItem := resolveOutputArtifactItem(builder, parentNode, globalVariables)
	if singleItem != "" {
		resolvedItems = append(resolvedItems, singleItem)
	}

	return resolvedItems
}

func extractOutputItemsFromChildren(
	builder *irBuilder,
	arrayNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) []string {
	var items []string
	for _, child := range arrayNode.ChildrenUnsafe() {
		resolved := resolveOutputArtifactItem(builder, child, globalVariables)
		if resolved != "" {
			items = append(items, resolved)
		}
	}
	return items
}

// resolveOutputArtifactItem resolves artifact outputs; only global variables are supported (not target parameters).
// Composite forms (path) must be dispatched before leaf forms.
func resolveOutputArtifactItem(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) string {
	if globNode := artifactItemGlobNode(node); globNode != nil {
		emitSemanticError(
			builder,
			globNode,
			ERROR_INVALID_GLOB,
			"glob() is not allowed in artifact outputs",
		)
		return ""
	}

	if pathNode := artifactItemPathNode(node); pathNode != nil {
		return evaluatePath(builder, pathNode, globalVariables)
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
