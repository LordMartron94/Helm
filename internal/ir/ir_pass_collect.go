package ir

import (
	"fmt"
	"helm/shared"
	"lingua/helm/artifacts"
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
		globalVariables: make(map[string]string),
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
		valueString := extractStringFromStringNode(
			builder,
			valueNode.FindFirstKind(artifacts.NodeStringLiteral),
			scope,
		)

		builder.globalVariables[identifierString] = valueString
	}
}

func validateTargetDependencies(builder *irBuilder) {
	for _, target := range builder.targets {
		for _, dep := range target.DependsOn {
			if _, exists := IRResolveTargetName(builder.targets, dep.TargetName); exists {
				continue
			}

			emitSemanticError(
				builder,
				dep.SourceNode,
				ERROR_UNDECLARED_TARGET,
				fmt.Sprintf("dependency '%s' refers to undeclared target", dep.TargetName),
			)
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
