package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"syntaxa"
)

func handleTargetExport(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	currentTarget *HelmTarget,
	state *targetParseState,
) {
	if state.exportDeclared {
		emitSemanticError(
			builder,
			node,
			ERROR_DUPLICATE_EXPORT,
			fmt.Sprintf("export block for target '%s' already declared", currentTarget.Name),
		)
		return
	}
	state.exportDeclared = true

	currentTarget.Export = make(map[string]HelmStringListExpr)

	var pendingKey string
	var pendingKeyNode *syntaxa.SyntaxaLSTNode[artifacts.Node]

	_ = node.WalkPre(func(cur *syntaxa.SyntaxaLSTNode[artifacts.Node]) (bool, bool) {
		switch cur.Kind() {
		case artifacts.NodeExportKey:
			keyStr := extractContentFromSingleTokenNode(builder, cur)
			if pendingKey != "" {
				emitSemanticError(
					builder,
					pendingKeyNode,
					ERROR_INVALID_EXPORT_VALUE,
					fmt.Sprintf("export key '%s' is missing a value", pendingKey),
				)
			}
			if _, exists := currentTarget.Export[keyStr]; exists {
				emitSemanticError(
					builder,
					cur,
					ERROR_DUPLICATE_EXPORT_KEY,
					fmt.Sprintf("export key '%s' declared multiple times", keyStr),
				)
				pendingKey = ""
				pendingKeyNode = nil
				return false, false
			}
			pendingKey = keyStr
			pendingKeyNode = cur
		case artifacts.NodeStringLiteral, artifacts.NodeStringListArray:
			if pendingKey == "" {
				return false, false
			}
			currentTarget.Export[pendingKey] = extractStringListFromNode(
				builder,
				cur,
				scope,
				false,
			)
			pendingKey = ""
			pendingKeyNode = nil
		}
		return false, false
	})

	if pendingKey != "" {
		emitSemanticError(
			builder,
			pendingKeyNode,
			ERROR_INVALID_EXPORT_VALUE,
			fmt.Sprintf("export key '%s' is missing a value", pendingKey),
		)
	}
}
