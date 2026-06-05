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
		case artifacts.NodeRunAbsPathCall:
			nameNode := child.FindDirectChildKind(artifacts.NodeRunAbsPathCallName)
			callName := extractContentFromSingleTokenNode(builder, nameNode)
			if callName != "abs_path" {
				emitSemanticError(
					builder,
					nameNode,
					ERROR_INVALID_PATH_EXPR,
					fmt.Sprintf("unknown run argv function '%s' (only abs_path is supported)", callName),
				)
				continue
			}
			strNode := child.FindFirstKind(artifacts.NodeStringLiteral)
			if strNode == nil {
				emitSemanticError(
					builder,
					child,
					ERROR_INVALID_PATH_EXPR,
					"abs_path requires a string literal argument",
				)
				continue
			}
			elements = append(elements, HelmRunArgvElement{
				AbsPath: extractStringFromStringNode(builder, strNode, scope),
			})
		case artifacts.NodeRunParameterRef:
			nameNode := child.FindDirectChildKind(artifacts.NodeRunParameterName)
			paramName := extractContentFromSingleTokenNode(builder, nameNode)
			if !resolveScopeHasParameter(scope, paramName) &&
				!resolveScopeHasLetBinding(scope, paramName) &&
				!globalVariableIsArtifactArray(scope, paramName) {
				emitSemanticError(
					builder,
					nameNode,
					ERROR_UNDECLARED_PARAMETER,
					fmt.Sprintf("run argv references undeclared binding '%s'", paramName),
				)
				continue
			}
			elements = append(elements, HelmRunArgvElement{ParamName: paramName})
		case artifacts.NodeStringListCollectCall:
			collect := extractCollectCall(builder, child, scope)
			if collect != nil {
				elements = append(elements, HelmRunArgvElement{Collect: collect})
			}
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
