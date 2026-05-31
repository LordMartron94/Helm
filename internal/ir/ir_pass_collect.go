package ir

import (
	"fmt"
	"helm/shared"
	"lingua/helm/artifacts"
	"path/filepath"
	"signal"
	"syntaxa"
)

func IRFromSyntax(
	filePath, sourceText string,
	rootNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	signalCtx *signal.SignalContext,
) HelmIR {
	signal.SignalContextPushSpan(signalCtx, shared.SemanticAnalysisSpanPhase)
	defer signal.SignalContextPopSpan(signalCtx)

	builder := &irBuilder{
		filePath:        filePath,
		sourceText:      sourceText,
		signalCtx:       signalCtx,
		seenAliases:     make(map[string]struct{}),
		globalVariables: make(map[string]HelmGlobalVariable),
		targets:         make(map[string]HelmTarget),
	}

	elements := rootNode.ChildrenUnsafe()
	for _, element := range elements {
		kind := element.Kind()

		switch kind {
		case artifacts.NodeVariableDeclaration:
			handleVariableDeclaration(builder, element)
		case artifacts.NodeTarget:
			handleTargetDeclaration(builder, element)
		default:
			panic(fmt.Errorf("interpreter error: unhandled child kind '%v'", kind))
		}
	}

	validateTargetDependencies(builder)

	return HelmIR{
		SourceDirectory: filepath.Dir(filePath),
		GlobalVariables: builder.globalVariables,
		Targets:         builder.targets,
		Succeeded:       !builder.hasEmittedError,
	}
}

func handleVariableDeclaration(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
) {
	signal.SignalContextPushSpan(builder.signalCtx, "Variable Declaration")
	defer signal.SignalContextPopSpan(builder.signalCtx)

	identifierNode := node.FindDirectChildKind(artifacts.NodeVariableName)
	identifierString := extractContentFromSingleTokenNode(builder, identifierNode)

	if _, exists := builder.globalVariables[identifierString]; exists {
		emitSemanticError(
			builder,
			identifierNode,
			ERROR_DUPLICATE_VARIABLE,
			fmt.Sprintf("variable '%s' has already been declared", identifierString),
		)
	} else {
		valueNode := node.FindDirectChildKind(artifacts.NodeVariableValue)
		scope := resolveScopeForGlobals(builder.globalVariables)

		if arrayNode := valueNode.FindFirstKind(artifacts.NodeStringArray); arrayNode != nil {
			values := extractStringsFromStringArrayNode(builder, arrayNode, scope)
			builder.globalVariables[identifierString] = HelmGlobalVariable{
				Kind:         HelmGlobalVarStringArray,
				StringValues: values,
			}
			return
		}

		stringNode := valueNode.FindFirstKind(artifacts.NodeStringLiteral)
		if stringNode == nil {
			emitSemanticError(
				builder,
				valueNode,
				ERROR_INVALID_VARIABLE_VALUE,
				fmt.Sprintf("variable '%s' must be a string literal or string array", identifierString),
			)
			return
		}

		builder.globalVariables[identifierString] = HelmGlobalVariable{
			Kind:        HelmGlobalVarString,
			StringValue: extractStringFromStringNode(builder, stringNode, scope),
		}
	}
}

func IRResolveTargetName(targets map[string]HelmTarget, name string) (string, bool) {
	if _, exists := targets[name]; exists {
		return name, true
	}

	for canonical, target := range targets {
		for _, alias := range target.Aliases {
			if alias == name {
				return canonical, true
			}
		}
	}

	return "", false
}
