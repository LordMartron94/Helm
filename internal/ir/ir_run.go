package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"syntaxa"
)

func extractRunCommandFromNode(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) HelmRunCommand {
	if arrayNode := node.FindFirstKind(artifacts.NodeRunArray); arrayNode != nil {
		return extractRunArgvFromArrayNode(builder, arrayNode, scope)
	}

	stringNode := findStringContentNode(node)
	return HelmRunCommand{
		String: extractStringFromStringNode(builder, stringNode, scope),
	}
}

func extractRunArgvFromArrayNode(
	builder *irBuilder,
	arrayNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) HelmRunCommand {
	var elements []HelmRunArgvElement

	for _, child := range arrayNode.ChildrenUnsafe() {
		switch child.Kind() {
		case artifacts.NodeStringLiteral:
			elements = append(elements, HelmRunArgvElement{
				Literal: extractStringFromStringNode(builder, child, scope),
			})
		case artifacts.NodeRunParameterRef:
			nameNode := child.FindDirectChildKind(artifacts.NodeRunParameterName)
			paramName := extractContentFromSingleTokenNode(builder, nameNode)
			if !resolveScopeHasParameter(scope, paramName) {
				emitSemanticError(
					builder,
					nameNode,
					ERROR_UNDECLARED_PARAMETER,
					fmt.Sprintf("run argv references undeclared parameter '%s'", paramName),
				)
				continue
			}
			elements = append(elements, HelmRunArgvElement{ParamName: paramName})
		}
	}

	if len(elements) == 0 {
		emitSemanticError(
			builder,
			arrayNode,
			ERROR_INVALID_VARIABLE_VALUE,
			"run array must contain at least one element",
		)
	}

	return HelmRunCommand{Argv: elements}
}
