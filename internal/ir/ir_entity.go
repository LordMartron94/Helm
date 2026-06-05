package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"signal"
	"syntaxa"
)

// entitySourceFile records the helm file path that declared an entity.
func entitySourceFile(builder *irBuilder) string {
	return builder.filePath
}

func handleWorkspaceDeclaration(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
) {
	signal.SignalContextPushSpan(builder.signalCtx, "Workspace Declaration")
	defer signal.SignalContextPopSpan(builder.signalCtx)

	if builder.workspaceDeclared {
		emitSemanticError(builder, node, ERROR_DUPLICATE_WORKSPACE, "workspace block may appear only once")
		return
	}
	builder.workspaceDeclared = true
	builder.workspaceGlobals = make(map[string]HelmGlobalVariable)
	builder.workspaceExcludes = nil

	body := node.FindDirectChildKind(artifacts.NodeWorkspaceBody)
	if body == nil {
		return
	}
	fileLocalScope := resolveScopeForGlobals(builder.globalVariables)
	for _, child := range body.ChildrenUnsafe() {
		switch child.Kind() {
		case artifacts.NodeWorkspaceGlobals:
			handleWorkspaceGlobalsBlock(builder, child, fileLocalScope)
		case artifacts.NodeWorkspaceExclude:
			handleWorkspaceExclude(builder, child, fileLocalScope)
		}
	}
}

func handleWorkspaceGlobalsBlock(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	fileLocalScope resolveScope,
) {
	body := node.FindDirectChildKind(artifacts.NodeWorkspaceGlobalsBody)
	if body == nil {
		return
	}
	for _, child := range body.ChildrenUnsafe() {
		if child.Kind() != artifacts.NodeWorkspaceGlobalAssignment {
			continue
		}
		workspaceApplyGlobalAssignment(builder, child, fileLocalScope)
	}
}

func workspaceApplyGlobalAssignment(
	builder *irBuilder,
	assignmentNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	fileLocalScope resolveScope,
) {
	keyNode := assignmentNode.FindDirectChildKind(artifacts.NodeWorkspaceGlobalKey)
	if keyNode == nil {
		return
	}
	key := extractContentFromSingleTokenNode(builder, keyNode)
	if _, exists := builder.workspaceGlobals[key]; exists {
		emitSemanticError(
			builder,
			keyNode,
			ERROR_DUPLICATE_VARIABLE,
			fmt.Sprintf("workspace global '%s' has already been declared", key),
		)
		return
	}

	valueNode := assignmentNode.FindDirectChildKind(artifacts.NodeVariableValue)
	if valueNode == nil {
		emitSemanticError(
			builder,
			assignmentNode,
			ERROR_INVALID_VARIABLE_VALUE,
			fmt.Sprintf("workspace global '%s' requires a value", key),
		)
		return
	}

	variable, ok := resolveGlobalValueFromValueNode(
		builder,
		valueNode,
		fileLocalScope,
		fmt.Sprintf("workspace global '%s'", key),
	)
	if !ok {
		return
	}
	builder.workspaceGlobals[key] = variable
}

func handleWorkspaceExclude(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	fileLocalScope resolveScope,
) {
	stringNode := node.FindFirstKind(artifacts.NodeStringLiteral)
	if stringNode == nil {
		emitSemanticError(builder, node, ERROR_INVALID_LABEL, "exclude requires a string path")
		return
	}
	path := extractStringFromStringNode(builder, stringNode, fileLocalScope)
	builder.workspaceExcludes = append(builder.workspaceExcludes, path)
}

func handleEntityDeclaration(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
) {
	signal.SignalContextPushSpan(builder.signalCtx, "Entity Declaration")
	defer signal.SignalContextPopSpan(builder.signalCtx)

	nameNode := node.FindDirectChildKind(artifacts.NodeEntityName)
	entityName := extractContentFromSingleTokenNode(builder, nameNode)

	label := HelmLabel{
		Path: entityName,
		Name: entityName,
	}

	entity := HelmEntity{
		Name:         entityName,
		Label:        label,
		InterfaceBag: make(map[string]HelmStringListExpr),
		Parameters:   make(map[string]HelmParameterValue),
		SourceFile:   entitySourceFile(builder),
	}

	body := node.FindDirectChildKind(artifacts.NodeEntityBody)
	handleEntityBody(builder, body, &entity)

	if entity.Kind == "bin" && len(entity.InterfaceBag) > 0 {
		emitSemanticError(
			builder,
			nameNode,
			ERROR_ENTITY_BIN_HAS_INTERFACE,
			fmt.Sprintf("entity '%s' is a binary leaf and cannot declare an interface block", entityName),
		)
	}

	key := HelmLabelCanonical(entity.Label)
	if _, exists := builder.entities[key]; exists {
		emitSemanticError(builder, nameNode, ERROR_DUPLICATE_ENTITY, fmt.Sprintf("entity '%s' already declared", key))
		return
	}
	if entity.AdapterName == "" {
		emitSemanticError(builder, nameNode, ERROR_ENTITY_MISSING_USE, fmt.Sprintf("entity '%s' must declare use <adapter>", entityName))
	}
	builder.entities[key] = entity
}

func handleEntityBody(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	entity *HelmEntity,
) {
	if node == nil {
		return
	}
	scope := resolveScopeForGlobals(builder.effectiveGlobals())

	for _, child := range node.ChildrenUnsafe() {
		switch child.Kind() {
		case artifacts.NodeEntityKind:
			stringNode := child.FindFirstKind(artifacts.NodeStringLiteral)
			if stringNode != nil {
				entity.Kind = extractStringFromStringNode(builder, stringNode, scope)
			}
		case artifacts.NodeEntityUse:
			handleEntityUse(builder, child, scope, entity)
		case artifacts.NodeEntityDeps:
			labelNodes := child.FindAllKind(artifacts.NodeLabelRef)
			for _, labelNode := range labelNodes {
				stringNode := labelNode.FindFirstKind(artifacts.NodeStringLiteral)
				if stringNode == nil {
					continue
				}
				text := extractStringFromStringNode(builder, stringNode, scope)
				label, ok := helmLabelFromStringLiteral(builder, text)
				if !ok {
					emitSemanticError(builder, labelNode, ERROR_INVALID_LABEL, fmt.Sprintf("invalid entity label %q", text))
					continue
				}
				entity.Deps = append(entity.Deps, label)
			}
		case artifacts.NodeEntityInterface:
			extractEntityInterfaceBag(builder, child, scope, entity)
		}
	}
}

func handleEntityUse(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	entity *HelmEntity,
) {
	adapterNode := node.FindDirectChildKind(artifacts.NodeEntityAdapterName)
	entity.AdapterName = extractContentFromSingleTokenNode(builder, adapterNode)

	optionsNode := node.FindDirectChildKind(artifacts.NodeEntityUseOptions)
	if optionsNode == nil {
		return
	}
	paramsNode := optionsNode.FindFirstKind(artifacts.NodeDependencyParameters)
	if paramsNode == nil {
		return
	}
	extractEntityAdapterParameters(builder, paramsNode, scope, entity)
}

func extractEntityAdapterParameters(
	builder *irBuilder,
	paramsNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	entity *HelmEntity,
) {
	dep := HelmTargetDependency{
		TargetName: entity.AdapterName,
		Parameters: entity.Parameters,
	}
	extractDependencyParameters(builder, paramsNode, scope, &dep)
	entity.Parameters = dep.Parameters
}

func extractEntityInterfaceBag(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	entity *HelmEntity,
) {
	var pendingKey string
	_ = node.WalkPre(func(cur *syntaxa.SyntaxaLSTNode[artifacts.Node]) (bool, bool) {
		switch cur.Kind() {
		case artifacts.NodeEntityInterfaceKey:
			pendingKey = extractContentFromSingleTokenNode(builder, cur)
		case artifacts.NodeStringLiteral, artifacts.NodeStringListArray:
			if pendingKey == "" {
				return false, false
			}
			entity.InterfaceBag[pendingKey] = extractStringListFromNode(builder, cur, scope, false)
			pendingKey = ""
		}
		return false, false
	})
}

func handleInterfaceDeclaration(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
) {
	nameNode := node.FindDirectChildKind(artifacts.NodeInterfaceName)
	name := extractContentFromSingleTokenNode(builder, nameNode)
	if _, exists := builder.interfaces[name]; exists {
		emitSemanticError(builder, nameNode, ERROR_DUPLICATE_INTERFACE, fmt.Sprintf("interface '%s' already declared", name))
		return
	}

	decl := HelmInterfaceDecl{Name: name}
	keysNode := node.FindFirstKind(artifacts.NodeInterfaceKeys)
	if keysNode != nil {
		decl.Keys = extractStringArrayLiterals(builder, keysNode, resolveScopeForGlobals(builder.effectiveGlobals()))
	}
	builder.interfaces[name] = decl
}

func handleAdapterDeclaration(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
) {
	nameNode := node.FindDirectChildKind(artifacts.NodeAdapterName)
	name := extractContentFromSingleTokenNode(builder, nameNode)
	if _, exists := builder.adapters[name]; exists {
		emitSemanticError(builder, nameNode, ERROR_DUPLICATE_ADAPTER, fmt.Sprintf("adapter '%s' already declared", name))
		return
	}

	decl := HelmAdapterDecl{
		Name: name,
		Env:  make(map[string]HelmStringListExpr),
	}

	paramsNode := node.FindDirectChildKind(artifacts.NodeTargetParams)
	if paramsNode != nil {
		for _, parameterNode := range paramsNode.FindAllKind(artifacts.NodeTargetParameter) {
			parameter := extractParameterFromNode(builder, parameterNode)
			if parameter.Name == "DEPENDENCIES" && parameter.Optional {
				parameter.DependencyList = true
			}
			decl.Parameters = append(decl.Parameters, parameter)
		}
	}

	baseScope := resolveScopeForGlobals(builder.effectiveGlobals())
	body := node.FindDirectChildKind(artifacts.NodeAdapterBody)
	if body == nil {
		builder.adapters[name] = decl
		return
	}

	if parseAdapterPhases(builder, body, &decl, baseScope) {
		builder.adapters[name] = decl
		return
	}

	children := body.ChildrenUnsafe()
	for _, child := range children {
		if child.Kind() == artifacts.NodeMatrixBlock {
			handleAdapterMatrix(builder, child, &decl, baseScope)
		}
	}

	matrixVariable := ""
	if decl.Matrix != nil {
		matrixVariable = decl.Matrix.VariableName
	}
	scope := resolveScopeForAdapter(baseScope, decl.Parameters, matrixVariable)
	matrixScope := resolveScopeForAdapter(baseScope, decl.Parameters, matrixVariable)

	inMatrixSection := decl.Matrix != nil
	matrixLegOutputsDeclared := false

	for _, child := range children {
		switch child.Kind() {
		case artifacts.NodeMatrixBlock:
			continue
		case artifacts.NodeAdapterOutputs:
			if inMatrixSection && !matrixLegOutputsDeclared {
				decl.MatrixLegOutputs = extractArtifactItemsFromPathArrayRoot(builder, child, matrixScope)
				matrixLegOutputsDeclared = true
				continue
			}
			decl.Outputs = extractArtifactItemsFromPathArrayRoot(builder, child, scope)
		case artifacts.NodeAdapterEnv:
			handleAdapterEnv(builder, child, scope, &decl)
			inMatrixSection = false
		case artifacts.NodeRunStatement:
			runCommand := extractRunCommandFromNode(builder, child, scope)
			if inMatrixSection {
				runCommand = extractRunCommandFromNode(builder, child, matrixScope)
				decl.MatrixRuns = append(decl.MatrixRuns, runCommand)
				continue
			}
			decl.Runs = append(decl.Runs, runCommand)
		default:
			emitSemanticError(
				builder,
				child,
				ERROR_INVALID_VARIABLE_VALUE,
				fmt.Sprintf("adapter '%s' contains an unsupported statement", name),
			)
		}
	}

	if decl.Matrix != nil && len(decl.MatrixRuns) == 0 {
		emitSemanticError(
			builder,
			nameNode,
			ERROR_EMPTY_MATRIX,
			fmt.Sprintf("adapter '%s' matrix section must declare at least one run command", name),
		)
	}
	if len(decl.Runs) == 0 && (decl.Matrix == nil || len(decl.MatrixRuns) == 0) {
		emitSemanticError(
			builder,
			nameNode,
			ERROR_INVALID_VARIABLE_VALUE,
			fmt.Sprintf("adapter '%s' must declare at least one run command", name),
		)
	}

	builder.adapters[name] = decl
}

func handleAdapterMatrix(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	decl *HelmAdapterDecl,
	baseScope resolveScope,
) {
	if decl.Matrix != nil {
		emitSemanticError(builder, node, ERROR_DUPLICATE_MATRIX, fmt.Sprintf("adapter '%s' already has a matrix block", decl.Name))
		return
	}

	identifierNode := node.FindDirectChildKind(artifacts.NodeMatrixIdentifier)
	if identifierNode == nil {
		emitSemanticError(builder, node, ERROR_EMPTY_MATRIX, "matrix block is missing a variable name")
		return
	}

	variableName := extractContentFromSingleTokenNode(builder, identifierNode)
	scope := resolveScopeForAdapter(baseScope, decl.Parameters, variableName)
	values := extractMatrixValues(builder, node, scope)
	if len(values) == 0 {
		emitSemanticError(
			builder,
			node,
			ERROR_EMPTY_MATRIX,
			fmt.Sprintf("matrix for adapter '%s' must have at least one value", decl.Name),
		)
		return
	}

	for _, value := range values {
		if value.Kind == MatrixValueParameterRef {
			paramFound := false
			for _, parameter := range decl.Parameters {
				if parameter.Name == value.ParameterName {
					paramFound = true
					break
				}
			}
			if !paramFound {
				emitSemanticError(
					builder,
					node,
					ERROR_UNDECLARED_PARAMETER,
					fmt.Sprintf(
						"adapter '%s' matrix references undeclared parameter '%s'",
						decl.Name,
						value.ParameterName,
					),
				)
			}
		}
	}

	decl.Matrix = &HelmMatrix{
		VariableName: variableName,
		Values:       values,
	}
}

func handleAdapterEnv(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	decl *HelmAdapterDecl,
) {
	var pendingKey string
	var pendingKeyNode *syntaxa.SyntaxaLSTNode[artifacts.Node]

	_ = node.WalkPre(func(cur *syntaxa.SyntaxaLSTNode[artifacts.Node]) (bool, bool) {
		switch cur.Kind() {
		case artifacts.NodeEnvKey:
			keyStr := extractContentFromSingleTokenNode(builder, cur)
			if pendingKey != "" {
				emitSemanticError(
					builder,
					pendingKeyNode,
					ERROR_INVALID_EXPORT_VALUE,
					fmt.Sprintf("adapter '%s' env variable '%s' is missing a value", decl.Name, pendingKey),
				)
			}
			pendingKey = keyStr
			pendingKeyNode = cur
		case artifacts.NodeStringListArray:
			if pendingKey == "" {
				return false, false
			}
			decl.Env[pendingKey] = extractStringListFromNode(builder, cur, scope, true)
			pendingKey = ""
			pendingKeyNode = nil
			return true, false
		case artifacts.NodeStringLiteral:
			if pendingKey == "" {
				return false, false
			}
			decl.Env[pendingKey] = extractStringListFromNode(builder, cur, scope, true)
			pendingKey = ""
			pendingKeyNode = nil
			return true, false
		}
		return false, false
	})

	if pendingKey != "" {
		emitSemanticError(
			builder,
			pendingKeyNode,
			ERROR_INVALID_EXPORT_VALUE,
			fmt.Sprintf("adapter '%s' env variable '%s' is missing a value", decl.Name, pendingKey),
		)
	}
}

func extractStringArrayLiterals(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) []string {
	var out []string
	for _, child := range node.FindAllKind(artifacts.NodeStringLiteral) {
		out = append(out, extractStringFromStringNode(builder, child, scope))
	}
	return out
}
