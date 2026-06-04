package ir

import (
	"lingua/helm/artifacts"
	"syntaxa"
)

func extractDynamicArtifactSequence(
	builder *irBuilder,
	parentNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) []HelmArtifactInput {
	if arrayNode := parentNode.FindFirstKind(artifacts.NodeDynamicArray); arrayNode != nil {
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
