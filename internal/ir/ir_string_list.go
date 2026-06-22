package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"syntaxa"
)

func extractStringListFromNode(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	allowParamAndCollect bool,
) HelmStringListExpr {
	if node == nil {
		return nil
	}

	switch node.Kind() {
	case artifacts.NodeStringLiteral:
		return HelmStringListExpr{{
			Kind:    StringListLiteral,
			Literal: extractStringFromStringNode(builder, node, scope),
		}}
	case artifacts.NodeStringListArray:
		return extractStringListFromArrayNode(builder, node, scope, allowParamAndCollect)
	default:
		emitSemanticError(
			builder,
			node,
			ERROR_INVALID_EXPORT_VALUE,
			"expected a string literal or string list array",
		)
		return nil
	}
}

func extractStringListFromArrayNode(
	builder *irBuilder,
	arrayNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	allowParamAndCollect bool,
) HelmStringListExpr {
	children := arrayNode.ChildrenUnsafe()
	out := make(HelmStringListExpr, 0, len(children))

	for _, child := range children {
		switch child.Kind() {
		case artifacts.NodeStringLiteral:
			out = append(out, HelmStringListElement{
				Kind:    StringListLiteral,
				Literal: extractStringFromStringNode(builder, child, scope),
			})
		case artifacts.NodeStringListParamRef:
			if !allowParamAndCollect {
				emitSemanticError(
					builder,
					child,
					ERROR_INVALID_EXPORT_VALUE,
					"param references are not allowed here",
				)
				continue
			}
			nameNode := child.FindDirectChildKind(artifacts.NodeStringListParamName)
			paramName := extractContentFromSingleTokenNode(builder, nameNode)
			if !resolveScopeAllowsParamRef(scope, paramName) {
				emitSemanticError(
					builder,
					nameNode,
					ERROR_UNDECLARED_PARAMETER,
					fmt.Sprintf("string list references undeclared parameter '%s'", paramName),
				)
				continue
			}
			out = append(out, HelmStringListElement{
				Kind:      StringListParamRef,
				ParamName: paramName,
			})
		case artifacts.NodeStringListCollectCall:
			if !allowParamAndCollect {
				emitSemanticError(
					builder,
					child,
					ERROR_INVALID_EXPORT_VALUE,
					"collect() is not allowed here",
				)
				continue
			}
			collect := extractCollectCall(builder, child, scope, false)
			if collect != nil {
				out = append(out, HelmStringListElement{
					Kind:    StringListCollect,
					Collect: collect,
				})
			}
		case artifacts.NodeStringListCollectClosureCall:
			if !allowParamAndCollect {
				emitSemanticError(
					builder,
					child,
					ERROR_INVALID_EXPORT_VALUE,
					"collect_closure() is not allowed here",
				)
				continue
			}
			collect := extractCollectCall(builder, child, scope, true)
			if collect != nil {
				out = append(out, HelmStringListElement{
					Kind:    StringListCollect,
					Collect: collect,
				})
			}
		case artifacts.NodeRel:
			relPath := extractRelPath(builder, child, scope)
			if relPath != "" {
				out = append(out, HelmStringListElement{
					Kind:    StringListRel,
					RelPath: relPath,
				})
			}
		case artifacts.NodeStringListFormatFlagsCall:
			if !allowParamAndCollect {
				emitSemanticError(
					builder,
					child,
					ERROR_INVALID_EXPORT_VALUE,
					"format_flags() is not allowed here",
				)
				continue
			}
			formatFlags := extractFormatFlagsCall(builder, child, scope)
			if formatFlags != nil {
				out = append(out, HelmStringListElement{
					Kind:        StringListFormatFlags,
					FormatFlags: formatFlags,
				})
			}
		}
	}

	if len(out) == 0 {
		emitSemanticError(
			builder,
			arrayNode,
			ERROR_INVALID_EXPORT_VALUE,
			"string list array must contain at least one element",
		)
	}

	return out
}

func extractCollectCall(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	closure bool,
) *HelmCollectExpr {
	callName := "collect()"
	if closure {
		callName = "collect_closure()"
	}

	argsNode := node.FindDirectChildKind(artifacts.NodeStringListCollectArgs)
	if argsNode == nil {
		emitSemanticError(
			builder,
			node,
			ERROR_INVALID_COLLECT_CALL,
			callName+" requires (dependencies_param, export_key)",
		)
		return nil
	}

	depsParamNode := argsNode.FindDirectChildKind(artifacts.NodeStringListCollectDepsParam)
	depsParam := extractContentFromSingleTokenNode(builder, depsParamNode)
	if !resolveScopeHasParameter(scope, depsParam) {
		emitSemanticError(
			builder,
			depsParamNode,
			ERROR_UNDECLARED_COLLECT_PARAM,
			fmt.Sprintf("%s references undeclared parameter '%s'", callName, depsParam),
		)
		return nil
	}

	keyNode := argsNode.FindFirstKind(artifacts.NodeStringLiteral)
	if keyNode == nil {
		emitSemanticError(
			builder,
			argsNode,
			ERROR_INVALID_COLLECT_CALL,
			callName+" requires a string literal export key",
		)
		return nil
	}

	return &HelmCollectExpr{
		DependenciesParam: depsParam,
		ExportKey:         extractStringFromStringNode(builder, keyNode, scope),
		Closure:           closure,
	}
}

func extractFormatFlagsCall(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) *HelmFormatFlagsExpr {
	argsNode := node.FindDirectChildKind(artifacts.NodeStringListFormatFlagsArgs)
	if argsNode == nil {
		emitSemanticError(
			builder,
			node,
			ERROR_INVALID_COLLECT_CALL,
			"format_flags() requires (prefix, inner)",
		)
		return nil
	}

	prefixNode := argsNode.FindFirstKind(artifacts.NodeStringLiteral)
	if prefixNode == nil {
		emitSemanticError(
			builder,
			argsNode,
			ERROR_INVALID_COLLECT_CALL,
			"format_flags() requires a string literal prefix",
		)
		return nil
	}

	var inner *HelmStringListElement
	for _, child := range argsNode.ChildrenUnsafe() {
		switch child.Kind() {
		case artifacts.NodeStringListCollectCall:
			collect := extractCollectCall(builder, child, scope, false)
			if collect != nil {
				inner = &HelmStringListElement{Kind: StringListCollect, Collect: collect}
			}
		case artifacts.NodeStringListCollectClosureCall:
			collect := extractCollectCall(builder, child, scope, true)
			if collect != nil {
				inner = &HelmStringListElement{Kind: StringListCollect, Collect: collect}
			}
		case artifacts.NodeStringListParamRef:
			nameNode := child.FindDirectChildKind(artifacts.NodeStringListParamName)
			paramName := extractContentFromSingleTokenNode(builder, nameNode)
			if !resolveScopeAllowsParamRef(scope, paramName) {
				emitSemanticError(
					builder,
					nameNode,
					ERROR_UNDECLARED_PARAMETER,
					fmt.Sprintf("format_flags() references undeclared parameter '%s'", paramName),
				)
				return nil
			}
			inner = &HelmStringListElement{Kind: StringListParamRef, ParamName: paramName}
		}
	}

	if inner == nil {
		emitSemanticError(
			builder,
			argsNode,
			ERROR_INVALID_COLLECT_CALL,
			"format_flags() requires collect(), collect_closure(), or param as second argument",
		)
		return nil
	}

	return &HelmFormatFlagsExpr{
		Prefix: extractStringFromStringNode(builder, prefixNode, scope),
		Inner:  *inner,
	}
}
