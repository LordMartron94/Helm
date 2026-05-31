package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"syntaxa"
)

func extractInputArtifactSequence(
	builder *irBuilder,
	parentNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) []HelmArtifactInput {
	if arrayNode := parentNode.FindFirstKind(artifacts.NodeInputArray); arrayNode != nil {
		return extractArtifactItemsFromChildren(builder, arrayNode, scope)
	}

	if items, ok := expandArtifactSequenceFromVariableRef(builder, parentNode, scope, "artifacts inputs"); ok {
		return items
	}

	item, ok := resolveArtifactItem(builder, parentNode, scope)
	if ok {
		return []HelmArtifactInput{item}
	}
	return nil
}

func extractOutputArtifactSequence(
	builder *irBuilder,
	parentNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) []HelmArtifactInput {
	if arrayNode := parentNode.FindFirstKind(artifacts.NodeOutputArray); arrayNode != nil {
		return extractArtifactItemsFromChildren(builder, arrayNode, scope)
	}

	if items, ok := expandArtifactSequenceFromVariableRef(builder, parentNode, scope, "artifacts outputs"); ok {
		return items
	}

	item, ok := resolveArtifactItem(builder, parentNode, scope)
	if ok {
		return []HelmArtifactInput{item}
	}
	return nil
}

func expandArtifactSequenceFromVariableRef(
	builder *irBuilder,
	parentNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	context string,
) ([]HelmArtifactInput, bool) {
	varNode := artifactItemVarRefNode(parentNode)
	if varNode == nil {
		return nil, false
	}

	varName := extractContentFromSingleTokenNode(builder, varNode)
	list, ok := resolveGlobalStringList(scope, varName)
	if !ok {
		if _, exists := scope.globals[varName]; exists {
			emitVariableNotScalar(builder, varNode, varName, context)
		} else {
			emitSemanticError(
				builder,
				varNode,
				ERROR_UNDECLARED_VARIABLE,
				fmt.Sprintf("use of undeclared variable '%s' in %s", varName, context),
			)
		}
		return nil, true
	}

	if len(list) == 1 {
		return nil, false
	}

	items := make([]HelmArtifactInput, len(list))
	for i, literal := range list {
		items[i] = HelmArtifactInput{Kind: ArtifactInputString, Literal: literal}
	}
	return items, true
}

func extractArtifactItemsFromChildren(
	builder *irBuilder,
	arrayNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) []HelmArtifactInput {
	var items []HelmArtifactInput
	for _, child := range arrayNode.ChildrenUnsafe() {
		item, ok := resolveArtifactItem(builder, child, scope)
		if ok {
			items = append(items, item)
		}
	}
	return items
}

func resolveArtifactItem(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) (HelmArtifactInput, bool) {
	if globNode := artifactItemGlobNode(node); globNode != nil {
		globIR, ok := evaluateGlob(builder, globNode, scope)
		if !ok {
			return HelmArtifactInput{}, false
		}
		return HelmArtifactInput{Kind: ArtifactInputGlob, Glob: globIR}, true
	}

	if pathNode := artifactItemPathNode(node); pathNode != nil {
		return HelmArtifactInput{
			Kind:    ArtifactInputString,
			Literal: evaluatePath(builder, pathNode, scope),
		}, true
	}

	if strNode := artifactItemStringNode(node); strNode != nil {
		return HelmArtifactInput{
			Kind:    ArtifactInputString,
			Literal: extractStringFromStringNode(builder, strNode, scope),
		}, true
	}

	if varNode := artifactItemVarRefNode(node); varNode != nil {
		varName := extractContentFromSingleTokenNode(builder, varNode)
		if text, ok := resolveGlobalString(scope, varName); ok {
			return HelmArtifactInput{Kind: ArtifactInputString, Literal: text}, true
		}
		if _, exists := scope.globals[varName]; exists {
			emitVariableNotScalar(builder, varNode, varName, "artifacts entry")
			return HelmArtifactInput{}, false
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
