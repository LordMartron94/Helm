package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"path/filepath"
	"syntaxa"
)

func evaluatePath(
	builder *irBuilder,
	pathNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) string {
	argsNode := pathNode.FindFirstKind(artifacts.NodePathArgs)
	if argsNode == nil {
		emitSemanticError(
			builder,
			pathNode,
			ERROR_INVALID_PATH,
			"path() is missing arguments",
		)
		return ""
	}

	children := argsNode.ChildrenUnsafe()
	if len(children) == 0 {
		emitSemanticError(
			builder,
			pathNode,
			ERROR_INVALID_PATH,
			"path() requires at least one element",
		)
		return ""
	}

	segments := make([]string, 0, len(children))
	for _, child := range children {
		segment, ok := resolvePathElement(builder, child, globalVariables)
		if !ok {
			return ""
		}
		segments = append(segments, segment)
	}

	return filepath.Join(segments...)
}

func resolvePathElement(
	builder *irBuilder,
	elementNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) (segment string, ok bool) {
	if varNode := elementNode.FindFirstKind(artifacts.NodeVariableReference); varNode != nil {
		varName := extractContentFromSingleTokenNode(builder, varNode)
		if val, exists := globalVariables[varName]; exists {
			return val, true
		}
		emitSemanticError(
			builder,
			varNode,
			ERROR_UNDECLARED_VARIABLE,
			fmt.Sprintf("use of undeclared variable '%s' in path()", varName),
		)
		return "", false
	}

	if strNode := elementNode.FindFirstKind(artifacts.NodeStringLiteral); strNode != nil {
		scope := resolveScopeForGlobals(globalVariables)
		return extractStringFromStringNode(builder, strNode, scope), true
	}

	emitSemanticError(
		builder,
		elementNode,
		ERROR_INVALID_PATH,
		"path() element must be a variable reference or string literal",
	)
	return "", false
}
