package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"signal"
	"syntaxa"
)

func handleConfigurationDeclaration(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
) {
	signal.SignalContextPushSpan(builder.signalCtx, "Configuration Declaration")
	defer signal.SignalContextPopSpan(builder.signalCtx)

	nameNode := node.FindDirectChildKind(artifacts.NodeConfigurationName)
	name := extractContentFromSingleTokenNode(builder, nameNode)
	if name == "" {
		emitSemanticError(builder, node, ERROR_UNKNOWN_CONFIGURATION, "configuration requires a name")
		return
	}

	if builder.configurations == nil {
		builder.configurations = make(map[string]HelmConfigurationDecl)
	}
	if _, exists := builder.configurations[name]; exists {
		emitSemanticError(
			builder,
			nameNode,
			ERROR_DUPLICATE_CONFIGURATION,
			fmt.Sprintf("configuration '%s' already declared", name),
		)
		return
	}

	decl := HelmConfigurationDecl{
		Name:    name,
		Globals: make(map[string]HelmGlobalVariable),
	}

	body := node.FindDirectChildKind(artifacts.NodeConfigurationBody)
	if body != nil {
		fileLocalScope := resolveScopeForGlobals(builder.effectiveGlobals())
		for _, child := range body.ChildrenUnsafe() {
			if child.Kind() != artifacts.NodeConfigurationGlobals {
				continue
			}
			globalsBody := child.FindDirectChildKind(artifacts.NodeWorkspaceGlobalsBody)
			if globalsBody == nil {
				continue
			}
			for _, assignment := range globalsBody.ChildrenUnsafe() {
				if assignment.Kind() != artifacts.NodeWorkspaceGlobalAssignment {
					continue
				}
				configurationApplyGlobalAssignment(builder, assignment, fileLocalScope, decl.Globals, name)
			}
		}
	}

	builder.configurations[name] = decl
}

func configurationApplyGlobalAssignment(
	builder *irBuilder,
	assignmentNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	fileLocalScope resolveScope,
	globals map[string]HelmGlobalVariable,
	configurationName string,
) {
	keyNode := assignmentNode.FindDirectChildKind(artifacts.NodeWorkspaceGlobalKey)
	if keyNode == nil {
		return
	}
	key := extractContentFromSingleTokenNode(builder, keyNode)
	if _, exists := globals[key]; exists {
		emitSemanticError(
			builder,
			keyNode,
			ERROR_DUPLICATE_VARIABLE,
			fmt.Sprintf("configuration '%s' global '%s' has already been declared", configurationName, key),
		)
		return
	}

	valueNode := assignmentNode.FindDirectChildKind(artifacts.NodeVariableValue)
	if valueNode == nil {
		emitSemanticError(
			builder,
			assignmentNode,
			ERROR_INVALID_VARIABLE_VALUE,
			fmt.Sprintf("configuration '%s' global '%s' requires a value", configurationName, key),
		)
		return
	}

	variable, ok := resolveGlobalValueFromValueNode(
		builder,
		valueNode,
		fileLocalScope,
		fmt.Sprintf("configuration '%s' global '%s'", configurationName, key),
	)
	if !ok {
		return
	}
	globals[key] = variable
}

func extractEntityLabelDependency(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) (HelmEntityDep, bool) {
	labelRef := node.FindFirstKind(artifacts.NodeLabelRef)
	if labelRef == nil {
		return HelmEntityDep{}, false
	}
	stringNode := labelRef.FindFirstKind(artifacts.NodeStringLiteral)
	if stringNode == nil {
		return HelmEntityDep{}, false
	}

	text := extractStringFromStringNode(builder, stringNode, scope)
	label, ok := helmLabelFromStringLiteral(builder, text)
	if !ok {
		emitSemanticError(builder, labelRef, ERROR_INVALID_LABEL, fmt.Sprintf("invalid entity label %q", text))
		return HelmEntityDep{}, false
	}

	dep := HelmEntityDep{Label: label}
	if label.Configuration != "" {
		dep.Configuration = label.Configuration
		label.Configuration = ""
		dep.Label = label
	}

	optionsNode := node.FindDirectChildKind(artifacts.NodeEntityDependencyOptions)
	if optionsNode != nil {
		override := extractEntityDependencyConfiguration(builder, optionsNode, scope)
		if override != "" {
			dep.Configuration = override
		}
	}

	return dep, true
}

func extractEntityDependencyConfiguration(
	builder *irBuilder,
	optionsNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) string {
	configNodes := optionsNode.FindAllKind(artifacts.NodeEntityDependencyConfiguration)
	if len(configNodes) == 0 {
		return ""
	}
	stringNode := configNodes[0].FindFirstKind(artifacts.NodeStringLiteral)
	if stringNode == nil {
		return ""
	}
	return extractStringFromStringNode(builder, stringNode, scope)
}
