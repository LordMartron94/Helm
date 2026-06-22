package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"path/filepath"
	"syntaxa"
)

func extractRelPath(
	builder *irBuilder,
	relNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) string {
	argsNode := relNode.FindFirstKind(artifacts.NodeRelArgs)
	if argsNode == nil {
		emitSemanticError(
			builder,
			relNode,
			ERROR_INVALID_PATH,
			"rel() is missing arguments",
		)
		return ""
	}

	children := argsNode.ChildrenUnsafe()
	if len(children) == 0 {
		emitSemanticError(
			builder,
			relNode,
			ERROR_INVALID_PATH,
			"rel() requires at least one element",
		)
		return ""
	}

	segments := make([]string, 0, len(children))
	for _, child := range children {
		segment, ok := resolveRelElement(builder, child, scope)
		if !ok {
			return ""
		}
		segments = append(segments, segment)
	}

	return filepath.Join(segments...)
}

func resolveRelElement(
	builder *irBuilder,
	elementNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) (segment string, ok bool) {
	if varNode := elementNode.FindFirstKind(artifacts.NodeVariableReference); varNode != nil {
		varName := extractContentFromSingleTokenNode(builder, varNode)
		if text, ok := resolveScopeVariableAsLiteral(scope, varName); ok {
			return text, true
		}
		if globalVariableIsArtifactArray(scope, varName) {
			emitVariableNotScalar(builder, varNode, varName, "rel() element")
			return "", false
		}
		if scope.permissiveGlobals {
			return "${" + varName + "}", true
		}
		emitSemanticError(
			builder,
			varNode,
			ERROR_UNDECLARED_VARIABLE,
			fmt.Sprintf("use of undeclared variable '%s' in rel()", varName),
		)
		return "", false
	}

	if strNode := elementNode.FindFirstKind(artifacts.NodeStringLiteral); strNode != nil {
		return extractStringFromStringNode(builder, strNode, scope), true
	}

	emitSemanticError(
		builder,
		elementNode,
		ERROR_INVALID_PATH,
		"rel() element must be a variable reference or string literal",
	)
	return "", false
}
