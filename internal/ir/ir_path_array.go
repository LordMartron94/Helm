package ir

import (
	"lingua/helm/artifacts"
	"syntaxa"
)

func extractArtifactItemsFromPathArrayRoot(
	builder *irBuilder,
	root *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) []HelmArtifactInput {
	var items []HelmArtifactInput

	for _, child := range root.ChildrenUnsafe() {
		if varNode := artifactItemVarRefNode(child); varNode != nil {
			varName := extractContentFromSingleTokenNode(builder, varNode)
			if nested, ok := resolveGlobalArtifactItems(scope, varName); ok {
				items = append(items, nested...)
				continue
			}
		}

		if item, ok := resolveArtifactItem(builder, child, scope); ok {
			items = append(items, item)
			continue
		}

		items = append(items, extractArtifactItemsFromPathArrayRoot(builder, child, scope)...)
	}

	return items
}
