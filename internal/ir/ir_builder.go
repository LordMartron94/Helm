package ir

import (
	"foundation/location"
	"helm/shared"
	"lingua/helm/artifacts"
	"signal"
	"syntaxa"
)

type irBuilder struct {
	filePath   string
	sourceText string

	signalCtx *signal.SignalContext

	hasEmittedError bool

	seenAliases      map[string]struct{}
	inheritedGlobals map[string]HelmGlobalVariable
	globalVariables  map[string]HelmGlobalVariable
	targets          map[string]HelmTarget

	workspaceDeclared bool
	workspaceGlobals  map[string]HelmGlobalVariable
	workspaceExcludes []string
	entities          map[string]HelmEntity
	interfaces        map[string]HelmInterfaceDecl
	adapters          map[string]HelmAdapterDecl
}

func emitSemanticError(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	errorID string,
	message string,
) {
	startL, startC, endL, endC, _ := syntaxa.LSTNodeLineSpanFromSource(node, builder.sourceText, 4)

	loc := location.LocationCreate("file", "", builder.filePath, "", "", map[string]any{
		"start_line":   startL,
		"start_column": startC,
		"end_line":     endL,
		"end_column":   endC,
	})

	signal.SignalContextBuild(builder.signalCtx, errorID, "ERROR").
		Location(&loc).
		Payload(shared.PhasePayloadKey, shared.SemanticAnalysisPhase).
		Payload(shared.MessagePayloadKey, message).
		Emit()

	builder.hasEmittedError = true
}

func (builder *irBuilder) effectiveGlobals() map[string]HelmGlobalVariable {
	if len(builder.inheritedGlobals) == 0 {
		return builder.globalVariables
	}
	merged := make(
		map[string]HelmGlobalVariable,
		len(builder.inheritedGlobals)+len(builder.globalVariables),
	)
	for key, value := range builder.inheritedGlobals {
		merged[key] = value
	}
	for key, value := range builder.globalVariables {
		merged[key] = value
	}
	return merged
}
