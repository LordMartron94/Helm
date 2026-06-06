package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"syntaxa"
)

var pathExprAllowedCalls = map[string]int{
	"map_ext":     3,
	"rebase_dir":  3,
	"join_prefix": 2,
}

func extractPathExprFromNode(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) (HelmPathExpr, bool) {
	if varOrCall := node.FindFirstKind(artifacts.NodePathVarOrCall); varOrCall != nil {
		return extractPathVarOrCall(builder, varOrCall, scope)
	}
	if strNode := node.FindFirstKind(artifacts.NodeStringLiteral); strNode != nil {
		return HelmPathExpr{
			Kind:    PathExprLiteral,
			Literal: extractStringFromStringNode(builder, strNode, scope),
		}, true
	}
	emitSemanticError(
		builder,
		node,
		ERROR_INVALID_PATH_EXPR,
		"path expression must be a function call, variable reference, or string literal",
	)
	return HelmPathExpr{}, false
}

func extractPathVarOrCall(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) (HelmPathExpr, bool) {
	nameNode := node.FindDirectChildKind(artifacts.NodePathVarOrCallName)
	name := extractContentFromSingleTokenNode(builder, nameNode)

	if argsNode := node.FindFirstKind(artifacts.NodePathCallArgs); argsNode != nil {
		return extractPathCall(builder, nameNode, name, argsNode, scope)
	}

	return resolvePathIdentifierRef(builder, nameNode, name, scope)
}

func extractPathCall(
	builder *irBuilder,
	nameNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	name string,
	argsNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) (HelmPathExpr, bool) {
	argCount, ok := pathExprAllowedCalls[name]
	if !ok {
		emitSemanticError(
			builder,
			nameNode,
			ERROR_INVALID_PATH_EXPR,
			fmt.Sprintf("unknown path function '%s'", name),
		)
		return HelmPathExpr{}, false
	}

	args := extractPathCallArgs(builder, argsNode, scope)
	if len(args) == 0 {
		emitSemanticError(
			builder,
			argsNode,
			ERROR_INVALID_PATH_EXPR,
			fmt.Sprintf("path function '%s' is missing arguments", name),
		)
		return HelmPathExpr{}, false
	}
	if len(args) != argCount {
		emitSemanticError(
			builder,
			argsNode,
			ERROR_INVALID_PATH_EXPR,
			fmt.Sprintf("path function '%s' requires %d arguments", name, argCount),
		)
		return HelmPathExpr{}, false
	}

	return HelmPathExpr{
		Kind:     PathExprCall,
		CallName: name,
		CallArgs: args,
	}, true
}

func extractPathCallArgs(
	builder *irBuilder,
	argsNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) []HelmPathExpr {
	children := argsNode.ChildrenUnsafe()
	args := make([]HelmPathExpr, 0, len(children))
	for _, child := range children {
		expr, ok := extractPathExprFromNode(builder, child, scope)
		if !ok {
			continue
		}
		args = append(args, expr)
	}
	return args
}

func resolvePathIdentifierRef(
	builder *irBuilder,
	nameNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	name string,
	scope resolveScope,
) (HelmPathExpr, bool) {
	if resolveScopeHasParameter(scope, name) {
		return HelmPathExpr{Kind: PathExprParamRef, Name: name}, true
	}
	if resolveScopeHasLetBinding(scope, name) {
		return HelmPathExpr{Kind: PathExprLetRef, Name: name}, true
	}
	if globalVariableIsArtifactArray(scope, name) {
		return HelmPathExpr{Kind: PathExprGlobalRef, Name: name}, true
	}
	emitSemanticError(
		builder,
		nameNode,
		ERROR_UNDECLARED_VARIABLE,
		fmt.Sprintf("path expression references undeclared binding '%s'", name),
	)
	return HelmPathExpr{}, false
}
