package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"syntaxa"
)

type paramValueArrayKind int

const (
	paramValueArrayStringList paramValueArrayKind = iota
	paramValueArrayDependencyList
)

func classifyParamValueArrayKind(
	arrayNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
) paramValueArrayKind {
	kind := paramValueArrayStringList
	_ = arrayNode.WalkPre(func(cur *syntaxa.SyntaxaLSTNode[artifacts.Node]) (bool, bool) {
		switch cur.Kind() {
		case artifacts.NodeTargetDependency:
			kind = paramValueArrayDependencyList
			return true, false
		case artifacts.NodeStringLiteral, artifacts.NodeStringListCollectCall:
			if kind != paramValueArrayDependencyList {
				kind = paramValueArrayStringList
			}
		}
		return false, false
	})
	return kind
}

func extractParamValueArrayParameter(
	builder *irBuilder,
	arrayNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) HelmParameterValue {
	switch classifyParamValueArrayKind(arrayNode) {
	case paramValueArrayDependencyList:
		return HelmParameterValue{
			Kind:         HelmParameterDependencyList,
			Dependencies: extractTargetArrayDependencies(builder, arrayNode, scope),
		}
	default:
		return HelmParameterValue{
			Kind:       HelmParameterStringList,
			StringList: extractStringListFromParamValueArray(builder, arrayNode, scope),
		}
	}
}

func extractStringListFromParamValueArray(
	builder *irBuilder,
	arrayNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) HelmStringListExpr {
	var out HelmStringListExpr

	_ = arrayNode.WalkPre(func(cur *syntaxa.SyntaxaLSTNode[artifacts.Node]) (bool, bool) {
		switch cur.Kind() {
		case artifacts.NodeStringLiteral:
			out = append(out, HelmStringListElement{
				Kind:    StringListLiteral,
				Literal: extractStringFromStringNode(builder, cur, scope),
			})
		case artifacts.NodeParamValueParamRef:
			nameNode := cur.FindDirectChildKind(artifacts.NodeParamValueParamName)
			paramName := extractContentFromSingleTokenNode(builder, nameNode)
			if !resolveScopeHasParameter(scope, paramName) {
				emitSemanticError(
					builder,
					nameNode,
					ERROR_UNDECLARED_PARAMETER,
					fmt.Sprintf("string list references undeclared parameter '%s'", paramName),
				)
				return false, false
			}
			out = append(out, HelmStringListElement{
				Kind:      StringListParamRef,
				ParamName: paramName,
			})
		case artifacts.NodeStringListCollectCall:
			collect := extractCollectCall(builder, cur, scope)
			if collect != nil {
				out = append(out, HelmStringListElement{
					Kind:    StringListCollect,
					Collect: collect,
				})
			}
		}
		return false, false
	})

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
