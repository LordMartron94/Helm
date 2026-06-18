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
	paramValueArrayArtifactItems
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
		case artifacts.NodeGlob:
			kind = paramValueArrayArtifactItems
		case artifacts.NodeStringLiteral, artifacts.NodeStringListCollectCall, artifacts.NodeStringListCollectClosureCall:
			if kind != paramValueArrayDependencyList && kind != paramValueArrayArtifactItems {
				kind = paramValueArrayStringList
			}
		default:
			if artifactItemPathNode(cur) != nil || artifactItemVarRefNode(cur) != nil {
				kind = paramValueArrayArtifactItems
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
	case paramValueArrayArtifactItems:
		return HelmParameterValue{
			Kind:          HelmParameterArtifactItems,
			ArtifactItems: extractArtifactItemsFromParamValueArray(builder, arrayNode, scope),
		}
	default:
		return HelmParameterValue{
			Kind:       HelmParameterStringList,
			StringList: extractStringListFromParamValueArray(builder, arrayNode, scope),
		}
	}
}

func extractArtifactItemsFromParamValueArray(
	builder *irBuilder,
	arrayNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) []HelmArtifactInput {
	var items []HelmArtifactInput
	for _, child := range arrayNode.ChildrenUnsafe() {
		items = append(items, extractArtifactItemsFromPathArrayRoot(builder, child, scope)...)
	}
	return items
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
			collect := extractCollectCall(builder, cur, scope, false)
			if collect != nil {
				out = append(out, HelmStringListElement{
					Kind:    StringListCollect,
					Collect: collect,
				})
			}
		case artifacts.NodeStringListCollectClosureCall:
			collect := extractCollectCall(builder, cur, scope, true)
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
