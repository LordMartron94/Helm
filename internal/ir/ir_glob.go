package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"syntaxa"
)

var supportedGlobKwargs = map[string]struct{}{
	"include":         {},
	"exclude":         {},
	"follow_symlinks": {},
	"recursive":       {},
	"types":           {},
}

func newHelmGlob(baseDirectory string) *HelmGlob {
	return &HelmGlob{
		BaseDirectory:  baseDirectory,
		FollowSymlinks: false,
		Recursive:      true,
		Types:          "files",
	}
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

	result := newHelmGlob(baseDirectory)
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

		if !applyGlobKwarg(builder, child, nameNode, kwargName, globalVariables, result) {
			return nil, false
		}
	}

	return result, true
}

func applyGlobKwarg(
	builder *irBuilder,
	kwargNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	nameNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	kwargName string,
	globalVariables map[string]string,
	result *HelmGlob,
) bool {
	switch kwargName {
	case "include", "exclude", "types":
		value, ok := extractGlobStringKwargValue(builder, kwargNode, nameNode, kwargName, globalVariables)
		if !ok {
			return false
		}
		switch kwargName {
		case "include":
			result.Include = value
		case "exclude":
			result.Exclude = value
		case "types":
			result.Types = value
		}
		return true
	case "follow_symlinks":
		parsed, ok := extractGlobBoolKwargValue(builder, kwargNode, nameNode, kwargName, globalVariables)
		if !ok {
			return false
		}
		result.FollowSymlinks = parsed
		return true
	case "recursive":
		parsed, ok := extractGlobBoolKwargValue(builder, kwargNode, nameNode, kwargName, globalVariables)
		if !ok {
			return false
		}
		result.Recursive = parsed
		return true
	default:
		return false
	}
}

func extractGlobStringKwargValue(
	builder *irBuilder,
	kwargNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	nameNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	kwargName string,
	globalVariables map[string]string,
) (string, bool) {
	if kwargNode.FindDirectChildKind(artifacts.NodeBoolean) != nil {
		emitSemanticError(
			builder,
			nameNode,
			ERROR_INVALID_GLOB,
			fmt.Sprintf("glob() keyword argument '%s' must be a string", kwargName),
		)
		return "", false
	}

	strNode := kwargNode.FindDirectChildKind(artifacts.NodeStringLiteral)
	if strNode == nil {
		emitSemanticError(
			builder,
			nameNode,
			ERROR_INVALID_GLOB,
			fmt.Sprintf("glob() keyword argument '%s' is missing a value", kwargName),
		)
		return "", false
	}

	scope := resolveScopeForGlobals(globalVariables)
	return extractStringFromStringNode(builder, strNode, scope), true
}

func extractGlobBoolKwargValue(
	builder *irBuilder,
	kwargNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	nameNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	kwargName string,
	globalVariables map[string]string,
) (bool, bool) {
	if boolNode := kwargNode.FindDirectChildKind(artifacts.NodeBoolean); boolNode != nil {
		return extractBooleanNode(builder, boolNode), true
	}

	strNode := kwargNode.FindDirectChildKind(artifacts.NodeStringLiteral)
	if strNode != nil {
		scope := resolveScopeForGlobals(globalVariables)
		value := extractStringFromStringNode(builder, strNode, scope)
		return parseGlobBoolStringKwarg(builder, nameNode, kwargName, value)
	}

	emitSemanticError(
		builder,
		nameNode,
		ERROR_INVALID_GLOB,
		fmt.Sprintf("glob() keyword argument '%s' must be true or false", kwargName),
	)
	return false, false
}

func parseGlobBoolStringKwarg(
	builder *irBuilder,
	nameNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	kwargName string,
	value string,
) (bool, bool) {
	switch value {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		emitSemanticError(
			builder,
			nameNode,
			ERROR_INVALID_GLOB,
			fmt.Sprintf("glob() keyword argument '%s' must be true or false", kwargName),
		)
		return false, false
	}
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
