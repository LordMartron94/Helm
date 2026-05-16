package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"syntaxa"
)

var supportedGlobKwargs = map[string]struct{}{
	"include": {},
	"exclude": {},
}

func evaluateGlob(
	builder *irBuilder,
	globNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) (*HelmGlob, bool) {
	argsNode := globNode.FindFirstKind(artifacts.NodeGlobArguments)
	if argsNode == nil {
		emitSemanticError(
			builder,
			globNode,
			ERROR_INVALID_GLOB,
			"glob() is missing arguments",
		)
		return nil, false
	}

	baseDirNode := argsNode.FindFirstKind(artifacts.NodeGlobBaseDirectory)
	if baseDirNode == nil {
		emitSemanticError(
			builder,
			globNode,
			ERROR_INVALID_GLOB,
			"glob() is missing a base directory",
		)
		return nil, false
	}

	baseDirectory, ok := resolveGlobBaseDirectory(builder, baseDirNode, globalVariables)
	if !ok {
		return nil, false
	}

	result := &HelmGlob{baseDirectory: baseDirectory}
	seenKwargs := map[string]struct{}{}

	for _, child := range argsNode.ChildrenUnsafe() {
		if child.Kind() != artifacts.NodeGlobKwarg {
			continue
		}

		nameNode := child.FindFirstKind(artifacts.NodeGlobKwargIdentifier)
		kwargName := extractContentFromSingleTokenNode(builder, nameNode)

		if _, seen := seenKwargs[kwargName]; seen {
			emitSemanticError(
				builder,
				nameNode,
				ERROR_DUPLICATE_GLOB_KWARG,
				fmt.Sprintf("glob() keyword argument '%s' is specified more than once", kwargName),
			)
			return nil, false
		}
		seenKwargs[kwargName] = struct{}{}

		if _, supported := supportedGlobKwargs[kwargName]; !supported {
			emitSemanticError(
				builder,
				nameNode,
				ERROR_UNKNOWN_GLOB_KWARG,
				fmt.Sprintf("glob() has unknown keyword argument '%s'", kwargName),
			)
			return nil, false
		}

		valueNode := child.FindFirstKind(artifacts.NodeStringLiteral)
		scope := resolveScopeForGlobals(globalVariables)
		value := extractStringFromStringNode(builder, valueNode, scope)

		switch kwargName {
		case "include":
			result.include = value
		case "exclude":
			result.exclude = value
		}
	}

	return result, true
}

func resolveGlobBaseDirectory(
	builder *irBuilder,
	baseDirNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) (string, bool) {
	if varNode := baseDirNode.FindFirstKind(artifacts.NodeVariableReference); varNode != nil {
		varName := extractContentFromSingleTokenNode(builder, varNode)
		if val, exists := globalVariables[varName]; exists {
			return val, true
		}
		emitSemanticError(
			builder,
			varNode,
			ERROR_UNDECLARED_VARIABLE,
			fmt.Sprintf("use of undeclared variable '%s' in glob()", varName),
		)
		return "", false
	}

	if strNode := baseDirNode.FindFirstKind(artifacts.NodeStringLiteral); strNode != nil {
		scope := resolveScopeForGlobals(globalVariables)
		return extractStringFromStringNode(builder, strNode, scope), true
	}

	emitSemanticError(
		builder,
		baseDirNode,
		ERROR_INVALID_GLOB,
		"glob() base directory must be a variable reference or string literal",
	)
	return "", false
}
