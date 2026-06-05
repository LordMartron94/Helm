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
		return extractArtifactItemsFromPathArrayRoot(builder, arrayNode, scope)
	}

	if items, ok := expandArtifactSequenceFromVariableRef(builder, parentNode, scope); ok {
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
		return extractArtifactItemsFromPathArrayRoot(builder, arrayNode, scope)
	}

	if items, ok := expandArtifactSequenceFromVariableRef(builder, parentNode, scope); ok {
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
) ([]HelmArtifactInput, bool) {
	varNode := artifactItemVarRefNode(parentNode)
	if varNode == nil {
		return nil, false
	}

	varName := extractContentFromSingleTokenNode(builder, varNode)
	if items, ok := resolveGlobalArtifactItems(scope, varName); ok {
		out := make([]HelmArtifactInput, len(items))
		copy(out, items)
		return out, true
	}
	if placeholder, ok := resolveScopeParameterPlaceholder(scope, varName); ok {
		return []HelmArtifactInput{{
			Kind:    ArtifactInputString,
			Literal: placeholder,
		}}, true
	}
	if resolveScopeHasLetBinding(scope, varName) {
		return []HelmArtifactInput{{
			Kind:    ArtifactInputLetRef,
			LetName: varName,
		}}, true
	}

	return nil, false
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
		if resolveScopeHasLetBinding(scope, varName) {
			return HelmArtifactInput{Kind: ArtifactInputLetRef, LetName: varName}, true
		}
		if text, ok := resolveScopeVariableAsLiteral(scope, varName); ok {
			return HelmArtifactInput{Kind: ArtifactInputString, Literal: text}, true
		}
		if globalVariableIsArtifactArray(scope, varName) {
			emitVariableNotScalar(builder, varNode, varName, "artifacts entry")
			return HelmArtifactInput{}, false
		}
		if scope.permissiveGlobals {
			return HelmArtifactInput{
				Kind:    ArtifactInputString,
				Literal: "${" + varName + "}",
			}, true
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
