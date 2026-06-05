package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"signal"
	"syntaxa"
)

func handleTargetDeclaration(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
) {
	signal.SignalContextPushSpan(builder.signalCtx, "Target Declaration")
	defer signal.SignalContextPopSpan(builder.signalCtx)

	targetNameNode := node.FindDirectChildKind(artifacts.NodeTargetIdentifier)
	targetNameString := extractContentFromSingleTokenNode(builder, targetNameNode)

	if _, exist := builder.targets[targetNameString]; exist {
		emitSemanticError(
			builder,
			targetNameNode,
			ERROR_DUPLICATE_TARGET,
			fmt.Sprintf("target '%s' has already been declared", targetNameString),
		)
		return
	}

	parameterNodes := node.FindDirectChildKind(artifacts.NodeTargetParams).FindAllKind(artifacts.NodeTargetParameter)
	parameters := make([]HelmTargetParameter, 0, len(parameterNodes))
	seenMap := map[string]struct{}{}

	for _, parameterNode := range parameterNodes {
		parameter := extractParameterFromNode(builder, parameterNode)

		if _, seen := seenMap[parameter.Name]; seen {
			emitSemanticError(
				builder,
				parameterNode,
				ERROR_DUPLICATE_TARGET_PARAMETER,
				fmt.Sprintf("parameter '%s' has already been declared", parameter.Name),
			)
		}

		parameters = append(parameters, parameter)
		seenMap[parameter.Name] = struct{}{}
	}

	currentTarget := &HelmTarget{
		Name:       targetNameString,
		Parameters: parameters,
	}

	body := node.FindDirectChildKind(artifacts.NodeTargetBody)
	handleTargetBody(builder, body, node, currentTarget)

	builder.targets[targetNameString] = *currentTarget
}

type targetParseState struct {
	helpDeclared        bool
	aliasesDeclared     bool
	matrixDeclared      bool
	artifactsDeclared   bool
	workDirDeclared     bool
	envDeclared         bool
	dependsDeclared     bool
	interactiveDeclared bool
	hiddenDeclared      bool
	dynamicDeclared     bool
}

func handleTargetBody(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	targetNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	currentTarget *HelmTarget,
) {
	state := &targetParseState{}
	currentTarget.Env = make(map[string]string)

	contentNodes := node.ChildrenUnsafe()

	for _, contentNode := range contentNodes {
		if contentNode.Kind() == artifacts.NodeMatrixBlock {
			handleTargetMatrix(builder, contentNode, currentTarget, state)
		}
	}

	matrixVariable := ""
	if currentTarget.Matrix != nil {
		matrixVariable = currentTarget.Matrix.VariableName
	}
	scope := resolveScopeForTargetWithMatrix(
		builder.globalVariables,
		currentTarget.Parameters,
		matrixVariable,
	)

	for _, contentNode := range contentNodes {
		kind := contentNode.Kind()

		switch kind {
		case artifacts.NodeMatrixBlock:
			continue
		case artifacts.NodeHelpStatement:
			handleTargetHelp(builder, contentNode, scope, currentTarget, state)
		case artifacts.NodeAliases:
			handleTargetAliases(builder, contentNode, scope, currentTarget, state)
		case artifacts.NodeArtifactsBlock:
			handleTargetArtifacts(builder, contentNode, scope, currentTarget, state)
		case artifacts.NodeWorkingDirectory:
			handleTargetWorkDir(builder, contentNode, scope, currentTarget, state)
		case artifacts.NodeEnvDeclaration:
			handleTargetEnv(builder, contentNode, scope, currentTarget, state)
		case artifacts.NodeRunStatement:
			handleTargetRun(builder, contentNode, scope, currentTarget)
		case artifacts.NodeConditional:
			handleTargetConditional(builder, contentNode, scope, currentTarget)
		case artifacts.NodeTargetDepends:
			handleTargetDependsOn(builder, contentNode, scope, currentTarget, state)
		case artifacts.NodeInteractiveStatement:
			handleTargetInteractive(builder, contentNode, currentTarget, state)
		case artifacts.NodeHiddenStatement:
			handleTargetHidden(builder, contentNode, currentTarget, state)
		default:
			panic(fmt.Errorf("interpreter error: unhandled child kind '%v'", kind))
		}
	}

	if currentTarget.Interactive && currentTarget.Matrix != nil {
		emitSemanticError(
			builder,
			targetNode,
			ERROR_INTERACTIVE_MATRIX,
			fmt.Sprintf("target '%s' cannot use interactive = true with a matrix block", currentTarget.Name),
		)
	}

	if !state.helpDeclared {
		emitSemanticError(
			builder,
			targetNode,
			ERROR_NO_HELP_TEXT,
			fmt.Sprintf("help text for target '%s' is missing and required", currentTarget.Name),
		)
	}

	if !state.artifactsDeclared {
		emitSemanticError(
			builder,
			targetNode,
			ERROR_NO_ARTIFACTS,
			fmt.Sprintf("artifacts block for target '%s' is missing and required", currentTarget.Name),
		)
	}
}

func handleTargetHidden(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	currentTarget *HelmTarget,
	state *targetParseState,
) {
	if state.hiddenDeclared {
		emitSemanticError(
			builder,
			node,
			ERROR_DUPLICATE_HIDDEN,
			fmt.Sprintf("hidden for target '%s' already declared", currentTarget.Name),
		)
		return
	}
	state.hiddenDeclared = true

	boolNode := node.FindFirstKind(artifacts.NodeBoolean)
	if boolNode == nil {
		emitSemanticError(
			builder,
			node,
			ERROR_INVALID_HIDDEN,
			fmt.Sprintf("hidden for target '%s' must be true or false", currentTarget.Name),
		)
		return
	}

	currentTarget.Hidden = extractBooleanNode(builder, boolNode)
}

func handleTargetInteractive(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	currentTarget *HelmTarget,
	state *targetParseState,
) {
	if state.interactiveDeclared {
		emitSemanticError(
			builder,
			node,
			ERROR_DUPLICATE_INTERACTIVE,
			fmt.Sprintf("interactive for target '%s' already declared", currentTarget.Name),
		)
		return
	}
	state.interactiveDeclared = true

	boolNode := node.FindFirstKind(artifacts.NodeBoolean)
	if boolNode == nil {
		emitSemanticError(
			builder,
			node,
			ERROR_INVALID_INTERACTIVE,
			fmt.Sprintf("interactive for target '%s' must be true or false", currentTarget.Name),
		)
		return
	}

	currentTarget.Interactive = extractBooleanNode(builder, boolNode)
	if currentTarget.Interactive && currentTarget.Matrix != nil {
		emitSemanticError(
			builder,
			node,
			ERROR_INTERACTIVE_MATRIX,
			fmt.Sprintf("target '%s' cannot use interactive = true with a matrix block", currentTarget.Name),
		)
	}
}

func handleTargetHelp(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	currentTarget *HelmTarget,
	state *targetParseState,
) {
	if state.helpDeclared {
		emitSemanticError(builder, node, ERROR_DUPLICATE_HELP_TEXT, fmt.Sprintf("help text for target '%s' already declared", currentTarget.Name))
		return
	}
	state.helpDeclared = true

	stringNode := findStringContentNode(node)
	currentTarget.HelpText = extractStringFromStringNode(builder, stringNode, scope)
}

func handleTargetAliases(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	currentTarget *HelmTarget,
	state *targetParseState,
) {
	if state.aliasesDeclared {
		emitSemanticError(builder, node, ERROR_DUPLICATE_ALIASES, fmt.Sprintf("aliases for target '%s' already declared", currentTarget.Name))
		return
	}
	state.aliasesDeclared = true

	stringArrayNode := node.FindFirstKind(artifacts.NodeStringArray)
	aliases := extractStringsFromStringArrayNode(builder, stringArrayNode, scope)

	for _, alias := range aliases {
		if _, seen := builder.seenAliases[alias]; seen {
			emitSemanticError(builder, stringArrayNode, ERROR_DUPLICATE_ALIAS, fmt.Sprintf("alias '%s' already used across targets", alias))
		} else {
			builder.seenAliases[alias] = struct{}{}
			currentTarget.Aliases = append(currentTarget.Aliases, alias)
		}
	}
}

func handleTargetDependsOn(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	currentTarget *HelmTarget,
	state *targetParseState,
) {
	if state.dependsDeclared {
		emitSemanticError(builder, node, ERROR_DUPLICATE_DEPENDS_ON, fmt.Sprintf("depends_on block for target '%s' already declared", currentTarget.Name))
		return
	}
	state.dependsDeclared = true

	dependencyNodes := node.FindAllKind(artifacts.NodeTargetDependency)
	for _, depNode := range dependencyNodes {
		currentTarget.DependsOn = append(currentTarget.DependsOn, extractDependency(builder, depNode, scope))
	}

	paramRefNodes := node.FindAllKind(artifacts.NodeDependsOnParameterRef)
	for _, refNode := range paramRefNodes {
		nameNode := refNode.FindDirectChildKind(artifacts.NodeDependencyParameterName)
		refName := extractContentFromSingleTokenNode(builder, nameNode)
		if !resolveScopeHasParameter(scope, refName) {
			emitSemanticError(
				builder,
				refNode,
				ERROR_UNDECLARED_VARIABLE,
				fmt.Sprintf(
					"depends_on param '%s' is not declared on target '%s'",
					refName,
					currentTarget.Name,
				),
			)
			continue
		}
		duplicate := false
		for _, existing := range currentTarget.DependsOnParamNames {
			if existing == refName {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		currentTarget.DependsOnParamNames = append(currentTarget.DependsOnParamNames, refName)
		irMarkTargetParameterDependencyListOnTarget(currentTarget, refName)
	}
}

func irMarkTargetParameterDependencyListOnTarget(target *HelmTarget, parameterName string) {
	for i := range target.Parameters {
		if target.Parameters[i].Name == parameterName {
			target.Parameters[i].DependencyList = true
			return
		}
	}
}

func irMarkTargetParameterDependencyList(
	targets map[string]HelmTarget,
	targetName string,
	parameterName string,
) {
	target, ok := targets[targetName]
	if !ok {
		return
	}
	irMarkTargetParameterDependencyListOnTarget(&target, parameterName)
	targets[targetName] = target
}

func extractTargetArrayDependencies(
	builder *irBuilder,
	arrayNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) []HelmTargetDependency {
	dependencyNodes := arrayNode.FindAllKind(artifacts.NodeTargetDependency)
	out := make([]HelmTargetDependency, 0, len(dependencyNodes))
	for _, depNode := range dependencyNodes {
		out = append(out, extractDependency(builder, depNode, scope))
	}
	return out
}

func extractDependency(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) HelmTargetDependency {
	dep := HelmTargetDependency{
		Parameters: make(map[string]HelmParameterValue),
	}

	targetNameNode := node.FindDirectChildKind(artifacts.NodeInvokeTarget)
	dep.TargetName = extractContentFromSingleTokenNode(builder, targetNameNode)
	dep.SourceNode = targetNameNode

	optionsNode := node.FindFirstKind(artifacts.NodeDependencyOptions)
	if optionsNode != nil {
		extractDependencyOptions(builder, optionsNode, scope, &dep)
	}

	return dep
}

func extractDependencyOptions(
	builder *irBuilder,
	optionsNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	dep *HelmTargetDependency,
) {
	children := optionsNode.ChildrenUnsafe()
	for i := 0; i < len(children); i++ {
		switch children[i].Kind() {
		case artifacts.NodeOptionalKW:
			if boolNode := dependencyOptionBooleanSuccessor(children, i); boolNode != nil {
				dep.Optional = extractBooleanNode(builder, boolNode)
			}
		case artifacts.NodeConfirmKW:
			if boolNode := dependencyOptionBooleanSuccessor(children, i); boolNode != nil {
				dep.Confirm = extractBooleanNode(builder, boolNode)
			}
		case artifacts.NodeDependencyParameters:
			extractDependencyParameters(builder, children[i], scope, dep)
		default:
			extractDependencyOptionAssignment(builder, children[i], dep)
		}
	}
}

func extractDependencyParameters(
	builder *irBuilder,
	paramsNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	dep *HelmTargetDependency,
) {
	var pendingKey string
	var pendingKeyNode *syntaxa.SyntaxaLSTNode[artifacts.Node]

	_ = paramsNode.WalkPre(func(cur *syntaxa.SyntaxaLSTNode[artifacts.Node]) (bool, bool) {
		switch cur.Kind() {
		case artifacts.NodeDependencyParameterName:
			keyStr := extractContentFromSingleTokenNode(builder, cur)
			if pendingKey != "" {
				emitSemanticError(
					builder,
					pendingKeyNode,
					ERROR_INVALID_VARIABLE_VALUE,
					fmt.Sprintf(
						"parameter '%s' for dependency '%s' must be a string literal or variable reference",
						pendingKey,
						dep.TargetName,
					),
				)
			}
			if _, exists := dep.Parameters[keyStr]; exists {
				emitSemanticError(
					builder,
					cur,
					ERROR_DUPLICATE_DEPENDENCY_PARAM,
					fmt.Sprintf("parameter '%s' declared multiple times for dependency '%s'", keyStr, dep.TargetName),
				)
				pendingKey = ""
				pendingKeyNode = nil
				return false, false
			}
			pendingKey = keyStr
			pendingKeyNode = cur
		case artifacts.NodeStringLiteral:
			if pendingKey == "" {
				return false, false
			}
			dep.Parameters[pendingKey] = HelmParameterValue{
				Kind:   HelmParameterScalar,
				Scalar: extractStringFromStringNode(builder, cur, scope),
			}
			pendingKey = ""
			pendingKeyNode = nil
		case artifacts.NodeTargetArray:
			if pendingKey == "" {
				return false, false
			}
			dep.Parameters[pendingKey] = HelmParameterValue{
				Kind:         HelmParameterDependencyList,
				Dependencies: extractTargetArrayDependencies(builder, cur, scope),
			}
			pendingKey = ""
			pendingKeyNode = nil
		case artifacts.NodeVariableReference:
			if pendingKey == "" {
				return false, false
			}
			refName := extractContentFromSingleTokenNode(builder, cur)
			if _, ok := scope.globals[refName]; ok {
				dep.Parameters[pendingKey] = HelmParameterValue{
					Kind:       HelmParameterGlobalRef,
					GlobalName: refName,
				}
			} else if resolveScopeHasParameter(scope, refName) {
				dep.Parameters[pendingKey] = HelmParameterValue{
					Kind:            HelmParameterTargetParamRef,
					TargetParamName: refName,
				}
			} else {
				emitSemanticError(
					builder,
					cur,
					ERROR_UNDECLARED_VARIABLE,
					fmt.Sprintf(
						"parameter '%s' references undeclared name '%s' for dependency '%s' (not a global or target parameter)",
						pendingKey,
						refName,
						dep.TargetName,
					),
				)
			}
			pendingKey = ""
			pendingKeyNode = nil
		}
		return false, false
	})

	if pendingKey != "" {
		emitSemanticError(
			builder,
			pendingKeyNode,
			ERROR_INVALID_VARIABLE_VALUE,
			fmt.Sprintf(
				"parameter '%s' for dependency '%s' must be a string literal or variable reference",
				pendingKey,
				dep.TargetName,
			),
		)
	}
}

func extractDependencyOptionAssignment(
	builder *irBuilder,
	assignmentNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	dep *HelmTargetDependency,
) {
	boolNode := assignmentNode.FindFirstKind(artifacts.NodeBoolean)
	if boolNode == nil {
		return
	}

	value := extractBooleanNode(builder, boolNode)
	if assignmentNode.FindFirstKind(artifacts.NodeOptionalKW) != nil {
		dep.Optional = value
	}
	if assignmentNode.FindFirstKind(artifacts.NodeConfirmKW) != nil {
		dep.Confirm = value
	}
}

func dependencyOptionBooleanSuccessor(
	children []*syntaxa.SyntaxaLSTNode[artifacts.Node],
	keywordIndex int,
) *syntaxa.SyntaxaLSTNode[artifacts.Node] {
	if keywordIndex+1 >= len(children) {
		return nil
	}
	next := children[keywordIndex+1]
	if next.Kind() == artifacts.NodeBoolean {
		return next
	}
	return nil
}

func extractArtifactsVolatile(
	builder *irBuilder,
	artifactsNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
) bool {
	volatileNode := artifactsNode.FindFirstKind(artifacts.NodeVolatile)
	if volatileNode == nil {
		return false
	}

	if boolNode := volatileNode.FindFirstKind(artifacts.NodeBoolean); boolNode != nil {
		return extractBooleanNode(builder, boolNode)
	}

	children := artifactsNode.ChildrenUnsafe()
	for i := 0; i < len(children); i++ {
		if children[i].Kind() != artifacts.NodeVolatile {
			continue
		}
		if boolNode := dependencyOptionBooleanSuccessor(children, i); boolNode != nil {
			return extractBooleanNode(builder, boolNode)
		}
	}
	return false
}

func handleTargetArtifacts(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	currentTarget *HelmTarget,
	state *targetParseState,
) {
	if state.artifactsDeclared {
		emitSemanticError(builder, node, ERROR_DUPLICATE_ARTIFACTS, fmt.Sprintf("artifacts block for target '%s' already declared", currentTarget.Name))
		return
	}
	state.artifactsDeclared = true

	artifactsIR := &HelmArtifacts{}

	if inputsNode := node.FindFirstKind(artifacts.NodeCacheInputs); inputsNode != nil {
		artifactsIR.Inputs = extractInputArtifactSequence(builder, inputsNode, scope)
	}

	if outputsNode := node.FindFirstKind(artifacts.NodeCacheOutputDirectory); outputsNode != nil {
		artifactsIR.Outputs = extractOutputArtifactSequence(builder, outputsNode, scope)
	}

	if dynamicNode := node.FindFirstKind(artifacts.NodeDynamic); dynamicNode != nil {
		if state.dynamicDeclared {
			emitSemanticError(
				builder,
				dynamicNode,
				ERROR_DUPLICATE_DYNAMIC,
				fmt.Sprintf("dynamic for target '%s' already declared", currentTarget.Name),
			)
		} else {
			state.dynamicDeclared = true
			artifactsIR.Dynamic = extractDynamicArtifactSequence(builder, dynamicNode, scope)
		}
	}

	artifactsIR.Volatile = extractArtifactsVolatile(builder, node)

	currentTarget.Artifacts = artifactsIR
}

func handleTargetWorkDir(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	currentTarget *HelmTarget,
	state *targetParseState,
) {
	if state.workDirDeclared {
		emitSemanticError(builder, node, ERROR_DUPLICATE_WORKDIR, fmt.Sprintf("workdir for target '%s' already declared", currentTarget.Name))
		return
	}
	state.workDirDeclared = true

	stringNode := node.FindFirstKind(artifacts.NodeStringLiteral)
	currentTarget.WorkDir = extractStringFromStringNode(builder, stringNode, scope)
}

func handleTargetEnv(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	currentTarget *HelmTarget,
	state *targetParseState,
) {
	if state.envDeclared {
		emitSemanticError(builder, node, ERROR_DUPLICATE_ENV, fmt.Sprintf("env block for target '%s' already declared", currentTarget.Name))
		return
	}
	state.envDeclared = true

	keyNodes := node.FindAllKind(artifacts.NodeEnvKey)
	valueNodes := node.FindAllKind(artifacts.NodeStringLiteral)

	for i, keyNode := range keyNodes {
		keyStr := extractContentFromSingleTokenNode(builder, keyNode)

		if _, exists := currentTarget.Env[keyStr]; exists {
			emitSemanticError(builder, keyNode, ERROR_DUPLICATE_ENV_KEY, fmt.Sprintf("environment variable '%s' declared multiple times", keyStr))
		} else {
			valStr := extractStringFromStringNode(builder, valueNodes[i], scope)
			currentTarget.Env[keyStr] = valStr
		}
	}
}

func handleTargetRun(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	currentTarget *HelmTarget,
) {
	stringNode := findStringContentNode(node)
	runText := extractStringFromStringNode(builder, stringNode, scope)
	currentTarget.Steps = append(currentTarget.Steps, HelmTargetStep{
		Kind: TargetStepRun,
		Run:  runText,
	})
}

func handleTargetConditional(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	currentTarget *HelmTarget,
) {
	condition := HelmCondition{}

	paramNode := node.FindFirstKind(artifacts.NodeConditionalParameter)
	paramName := extractContentFromSingleTokenNode(builder, paramNode)
	condition.Parameter = paramName

	paramExists := false
	for _, p := range currentTarget.Parameters {
		if p.Name == paramName {
			paramExists = true
			break
		}
	}

	if !paramExists {
		emitSemanticError(
			builder,
			paramNode,
			ERROR_UNDECLARED_PARAMETER,
			fmt.Sprintf("condition references undeclared parameter '%s'", paramName),
		)
	}

	conditionNode := node.FindFirstKind(artifacts.NodeCondition)
	tokenKind := artifacts.Token(conditionNode.Tokens()[0].Token)

	switch tokenKind {
	case artifacts.TokKWDefined:
		condition.ConditionType = ConditionDefined
	case artifacts.TokKWNotDefined:
		condition.ConditionType = ConditionNotDefined
	case artifacts.TokKWEquals:
		condition.ConditionType = ConditionEquals
	case artifacts.TokKWNotEquals:
		condition.ConditionType = ConditionNotEquals
	}

	if condition.ConditionType == ConditionEquals || condition.ConditionType == ConditionNotEquals {
		targetValueNode := node.FindFirstKind(artifacts.NodeConditionalTarget)
		if strNode := targetValueNode.FindFirstKind(artifacts.NodeStringLiteral); strNode != nil {
			condition.TargetValue = extractStringFromStringNode(builder, strNode, scope)
		} else if intNode := targetValueNode.FindFirstKind(artifacts.NodeNumber); intNode != nil {
			condition.TargetValue = extractContentFromSingleTokenNode(builder, intNode)
		}
	}

	runNodes := node.FindAllKind(artifacts.NodeRunStatement)
	for _, runNode := range runNodes {
		stringNode := findStringContentNode(runNode)
		runText := extractStringFromStringNode(builder, stringNode, scope)
		condition.Runs = append(condition.Runs, runText)
	}

	currentTarget.Steps = append(currentTarget.Steps, HelmTargetStep{
		Kind: TargetStepWhen,
		When: &condition,
	})
}
