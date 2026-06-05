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
	inheritedGlobals map[string]HelmGlobalVariable,
) HelmIR {
	signal.SignalContextPushSpan(signalCtx, shared.SemanticAnalysisSpanPhase)
	defer signal.SignalContextPopSpan(signalCtx)

	builder := &irBuilder{
		filePath:         filePath,
		sourceText:       sourceText,
		signalCtx:        signalCtx,
		seenAliases:      make(map[string]struct{}),
		inheritedGlobals: inheritedGlobals,
		globalVariables:  make(map[string]HelmGlobalVariable),
		targets:          make(map[string]HelmTarget),
		entities:         make(map[string]HelmEntity),
		interfaces:       make(map[string]HelmInterfaceDecl),
		adapters:         make(map[string]HelmAdapterDecl),
	}

	elements := rootNode.ChildrenUnsafe()
	for _, element := range elements {
		if element.Kind() == artifacts.NodeVariableDeclaration {
			handleVariableDeclaration(builder, element)
		}
	}

	for _, element := range elements {
		kind := element.Kind()

		switch kind {
		case artifacts.NodeVariableDeclaration:
			continue
		case artifacts.NodeTarget:
			handleTargetDeclaration(builder, element)
		case artifacts.NodeWorkspace:
			handleWorkspaceDeclaration(builder, element)
		case artifacts.NodeEntity:
			handleEntityDeclaration(builder, element)
		case artifacts.NodeInterfaceDecl:
			handleInterfaceDeclaration(builder, element)
		case artifacts.NodeAdapterDecl:
			handleAdapterDeclaration(builder, element)
		default:
			panic(fmt.Errorf("interpreter error: unhandled child kind '%v'", kind))
		}
	}

	validateTargetDependencies(builder)

	var workspace *HelmWorkspace
	if builder.workspaceDeclared {
		workspace = &HelmWorkspace{
			Globals:  builder.workspaceGlobals,
			Excludes: append([]string(nil), builder.workspaceExcludes...),
		}
	}

	mode := HelmModeLegacy
	if builder.workspaceDeclared || len(builder.entities) > 0 {
		mode = HelmModeWorkspace
	}

	return HelmIR{
		SourceDirectory: filepath.Dir(filePath),
		GlobalVariables: builder.globalVariables,
		Targets:         builder.targets,
		Workspace:       workspace,
		Entities:        builder.entities,
		Interfaces:      builder.interfaces,
		Adapters:        builder.adapters,
		Mode:            mode,
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
		return
	}
	if _, exists := builder.inheritedGlobals[identifierString]; exists {
		emitSemanticError(
			builder,
			identifierNode,
			ERROR_DUPLICATE_VARIABLE,
			fmt.Sprintf("variable '%s' is already declared in the workspace", identifierString),
		)
		return
	}

	valueNode := node.FindDirectChildKind(artifacts.NodeVariableValue)
	scope := resolveScopeForGlobals(builder.effectiveGlobals())

	variable, ok := resolveGlobalValueFromValueNode(
		builder,
		valueNode,
		scope,
		fmt.Sprintf("variable '%s'", identifierString),
	)
	if !ok {
		return
	}
	builder.globalVariables[identifierString] = variable
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
