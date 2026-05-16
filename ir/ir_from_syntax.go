package ir

import (
	"fmt"
	"foundation/location"
	"helm/shared"
	"lingua/helm/artifacts"
	"signal"
	"strconv"
	"strings"
	"syntaxa"
)

const (
	ERROR_DUPLICATE_VARIABLE  string = "VAR_001"
	ERROR_UNDECLARED_VARIABLE string = "VAR_002"

	ERROR_DUPLICATE_TARGET           string = "TARGET_001"
	ERROR_UNDECLARED_TARGET          string = "TARGET_002"
	ERROR_DUPLICATE_TARGET_PARAMETER string = "TARGET_003"
	ERROR_DUPLICATE_HELP_TEXT        string = "TARGET_004"
	ERROR_NO_HELP_TEXT               string = "TARGET_005"
	ERROR_DUPLICATE_ALIAS            string = "TARGET_006"
	ERROR_DUPLICATE_ALIASES          string = "TARGET_007"
	ERROR_NO_ARTIFACTS               string = "TARGET_008"
	ERROR_DUPLICATE_ARTIFACTS        string = "TARGET_009"
	ERROR_DUPLICATE_WORKDIR          string = "TARGET_010"
	ERROR_DUPLICATE_ENV              string = "TARGET_011"
	ERROR_DUPLICATE_DEPENDS_ON       string = "TARGET_012"
	ERROR_DUPLICATE_ENV_KEY          string = "TARGET_013"

	ERROR_UNDECLARED_PARAMETER string = "COND_001"
)

type HelmConditionType int

const (
	ConditionDefined HelmConditionType = iota
	ConditionNotDefined
	ConditionEquals
	ConditionNotEquals
)

type HelmCondition struct {
	ConditionType HelmConditionType
	Parameter     string
	TargetValue   string // Only used for Equals/NotEquals
	Runs          []string
}

type HelmIR struct {
	globalVariables map[string]string
	targets         map[string]HelmTarget

	succeeded bool
}

type HelmTarget struct {
	name       string
	aliases    []string
	helpText   string
	parameters []HelmTargetParameter

	workDir string
	env     map[string]string
	runs    []string

	dependsOn    []HelmTargetDependency
	artifacts    *HelmArtifacts
	conditionals []HelmCondition
}

type HelmTargetParameter struct {
	name     string
	optional bool
}

type HelmTargetDependency struct {
	targetName string
	optional   bool
	confirm    bool
}

type HelmArtifacts struct {
	volatile bool
	inputs   []string
	outputs  []string
}

func (h *HelmIR) Success() bool {
	return h.succeeded
}

func IRFromSyntax(
	filePath, sourceText string,
	rootNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	signalCtx *signal.SignalContext,
) HelmIR {
	signal.SignalContextPushSpan(signalCtx, shared.SemanticAnalysisSpanPhase)
	defer signal.SignalContextPopSpan(signalCtx)

	globalVariables := make(map[string]string)
	targets := make(map[string]HelmTarget)

	builder := &irBuilder{
		filePath:    filePath,
		sourceText:  sourceText,
		signalCtx:   signalCtx,
		seenAliases: make(map[string]struct{}),
	}

	elements := rootNode.ChildrenUnsafe()
	for _, element := range elements {
		kind := element.Kind()

		switch kind {
		case artifacts.NodeVariableDeclaration:
			handleVariableDeclaration(builder, element, globalVariables)
		case artifacts.NodeTarget:
			handleTargetDeclaration(builder, element, globalVariables, targets)
		default:
			panic(fmt.Errorf("interpreter error: unhandled child kind '%v'", kind))
		}
	}

	return HelmIR{
		globalVariables: globalVariables,
		targets:         targets,
		succeeded:       !builder.hasEmittedError,
	}
}

// ---------------------------------------------------------------- BUILDER

type irBuilder struct {
	filePath   string
	sourceText string

	signalCtx *signal.SignalContext

	hasEmittedError bool

	seenAliases map[string]struct{}
}

func handleVariableDeclaration(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) {
	signal.SignalContextPushSpan(builder.signalCtx, "Variable Declaration")
	defer signal.SignalContextPopSpan(builder.signalCtx)

	identifierNode := node.FindDirectChildKind(artifacts.NodeVariableName)
	identifierString := extractContentFromSingleTokenNode(builder, identifierNode)

	if _, exists := globalVariables[identifierString]; exists {
		emitSemanticError(builder, identifierNode, ERROR_DUPLICATE_VARIABLE, fmt.Sprintf("variable '%s' has already been declared", identifierString))
	} else {
		valueNode := node.FindDirectChildKind(artifacts.NodeVariableValue)
		valueString := extractStringFromStringNode(
			builder,
			valueNode.FindFirstKind(artifacts.NodeStringLiteral),
			globalVariables,
		)

		globalVariables[identifierString] = valueString
	}
}

func handleTargetDeclaration(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
	targets map[string]HelmTarget,
) {
	signal.SignalContextPushSpan(builder.signalCtx, "Target Declaration")
	defer signal.SignalContextPopSpan(builder.signalCtx)

	targetNameNode := node.FindDirectChildKind(artifacts.NodeTargetIdentifier)
	targetNameString := extractContentFromSingleTokenNode(builder, targetNameNode)

	if _, exist := targets[targetNameString]; exist {
		emitSemanticError(builder, targetNameNode, ERROR_DUPLICATE_TARGET, fmt.Sprintf("target '%s' has already been declared", targetNameString))
	} else {
		parameterNodes := node.FindDirectChildKind(artifacts.NodeTargetParams).FindAllKind(artifacts.NodeTargetParameter)
		parameters := make([]HelmTargetParameter, len(parameterNodes))

		seenMap := map[string]struct{}{}

		for _, parameterNode := range parameterNodes {
			parameter := extractParameterFromNode(builder, parameterNode)

			if _, seen := seenMap[parameter.name]; seen {
				emitSemanticError(builder, parameterNode, ERROR_DUPLICATE_TARGET_PARAMETER, fmt.Sprintf("parameter '%s' has already been declared", parameter.name))
			}

			parameters = append(parameters, parameter)
			seenMap[parameter.name] = struct{}{}
		}

		currentTarget := &HelmTarget{
			name:       targetNameString,
			parameters: parameters,
		}

		body := node.FindDirectChildKind(artifacts.NodeTargetBody)
		handleTargetBody(builder, body, globalVariables, targets, node, currentTarget)

		targets[targetNameString] = *currentTarget
	}
}

/*
ARTIFACTS_BLOCK | CONDITIONAL_BLOCK | DEPENDS_ON
| RUN | WORK_DIR | ENV_DECLARATION
*/

type targetParseState struct {
	helpDeclared      bool
	aliasesDeclared   bool
	artifactsDeclared bool
	workDirDeclared   bool
	envDeclared       bool
	dependsDeclared   bool
}

func handleTargetBody(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
	targets map[string]HelmTarget,
	targetNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	currentTarget *HelmTarget,
) {
	state := &targetParseState{}

	currentTarget.env = make(map[string]string)

	contentNodes := node.ChildrenUnsafe()

	for _, contentNode := range contentNodes {
		kind := contentNode.Kind()

		switch kind {
		case artifacts.NodeHelpStatement:
			handleTargetHelp(builder, contentNode, globalVariables, currentTarget, state)
		case artifacts.NodeAliases:
			handleTargetAliases(builder, contentNode, globalVariables, currentTarget, state)
		case artifacts.NodeArtifactsBlock:
			handleTargetArtifacts(builder, contentNode, globalVariables, currentTarget, state)
		case artifacts.NodeWorkingDirectory:
			handleTargetWorkDir(builder, contentNode, globalVariables, currentTarget, state)
		case artifacts.NodeEnvDeclaration:
			handleTargetEnv(builder, contentNode, globalVariables, currentTarget, state)
		case artifacts.NodeRunStatement:
			handleTargetRun(builder, contentNode, globalVariables, currentTarget)
		case artifacts.NodeConditional:
			handleTargetConditional(builder, contentNode, globalVariables, currentTarget)
		case artifacts.NodeTargetDepends:
			handleTargetDependsOn(builder, contentNode, currentTarget, state)
		default:
			panic(fmt.Errorf("interpreter error: unhandled child kind '%v'", kind))
		}
	}

	if !state.helpDeclared {
		emitSemanticError(builder, targetNode, ERROR_NO_HELP_TEXT, fmt.Sprintf("help text for target '%s' is missing and required", currentTarget.name))
	}

	if !state.artifactsDeclared {
		emitSemanticError(builder, targetNode, ERROR_NO_ARTIFACTS, fmt.Sprintf("artifacts block for target '%s' is missing and required", currentTarget.name))
	}
}

func handleTargetHelp(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
	currentTarget *HelmTarget,
	state *targetParseState,
) {
	if state.helpDeclared {
		emitSemanticError(builder, node, ERROR_DUPLICATE_HELP_TEXT, fmt.Sprintf("help text for target '%s' already declared", currentTarget.name))
		return
	}
	state.helpDeclared = true

	stringNode := node.FindFirstKind(artifacts.NodeStringLiteral)
	currentTarget.helpText = extractStringFromStringNode(builder, stringNode, globalVariables)
}

func handleTargetAliases(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
	currentTarget *HelmTarget,
	state *targetParseState,
) {
	if state.aliasesDeclared {
		emitSemanticError(builder, node, ERROR_DUPLICATE_ALIASES, fmt.Sprintf("aliases for target '%s' already declared", currentTarget.name))
		return
	}
	state.aliasesDeclared = true

	stringArrayNode := node.FindFirstKind(artifacts.NodeStringArray)
	aliases := extractStringsFromStringArrayNode(builder, stringArrayNode, globalVariables)

	for _, alias := range aliases {
		if _, seen := builder.seenAliases[alias]; seen {
			emitSemanticError(builder, stringArrayNode, ERROR_DUPLICATE_ALIAS, fmt.Sprintf("alias '%s' already used across targets", alias))
		} else {
			builder.seenAliases[alias] = struct{}{}
			currentTarget.aliases = append(currentTarget.aliases, alias)
		}
	}
}

func handleTargetDependsOn(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	currentTarget *HelmTarget,
	state *targetParseState,
) {
	if state.dependsDeclared {
		emitSemanticError(builder, node, ERROR_DUPLICATE_DEPENDS_ON, fmt.Sprintf("depends_on block for target '%s' already declared", currentTarget.name))
		return
	}
	state.dependsDeclared = true

	dependencyNodes := node.FindAllKind(artifacts.NodeTargetDependency)
	for _, depNode := range dependencyNodes {
		currentTarget.dependsOn = append(currentTarget.dependsOn, extractDependency(builder, depNode))
	}
}

func extractDependency(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
) HelmTargetDependency {
	dep := HelmTargetDependency{}

	targetNameNode := node.FindDirectChildKind(artifacts.NodeInvokeTarget)
	dep.targetName = extractContentFromSingleTokenNode(builder, targetNameNode)

	optionsNode := node.FindDirectChildKind(artifacts.NodeDependencyOptions)
	if optionsNode != nil {
		extractDependencyOptions(builder, optionsNode, &dep)
	}

	return dep
}

func extractDependencyOptions(
	builder *irBuilder,
	optionsNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	dep *HelmTargetDependency,
) {
	tokens := optionsNode.Tokens()
	boolNodes := optionsNode.FindAllKind(artifacts.NodeBoolean)
	boolIndex := 0

	for _, tk := range tokens {
		switch artifacts.Token(tk.Token) {
		case artifacts.TokKWConfirm:
			if boolIndex < len(boolNodes) {
				dep.confirm = extractBooleanNode(builder, boolNodes[boolIndex])
				boolIndex++
			}
		case artifacts.TokKWOptional:
			if boolIndex < len(boolNodes) {
				dep.optional = extractBooleanNode(builder, boolNodes[boolIndex])
				boolIndex++
			}
		}
	}
}

func handleTargetArtifacts(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
	currentTarget *HelmTarget,
	state *targetParseState,
) {
	if state.artifactsDeclared {
		emitSemanticError(builder, node, ERROR_DUPLICATE_ARTIFACTS, fmt.Sprintf("artifacts block for target '%s' already declared", currentTarget.name))
		return
	}
	state.artifactsDeclared = true

	artifactsIR := &HelmArtifacts{}

	if inputsNode := node.FindFirstKind(artifacts.NodeCacheInputs); inputsNode != nil {
		artifactsIR.inputs = extractArtifactItemSequence(builder, inputsNode, globalVariables)
	}

	if outputsNode := node.FindFirstKind(artifacts.NodeCacheOutputDirectory); outputsNode != nil {
		artifactsIR.outputs = extractArtifactItemSequence(builder, outputsNode, globalVariables)
	}

	if volatileNode := node.FindFirstKind(artifacts.NodeVolatile); volatileNode != nil {
		boolNode := volatileNode.FindFirstKind(artifacts.NodeBoolean)
		artifactsIR.volatile = extractBooleanNode(builder, boolNode)
	}

	currentTarget.artifacts = artifactsIR
}

func extractArtifactItemSequence(
	builder *irBuilder,
	parentNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) []string {
	var resolvedItems []string

	// Check if it's an array block
	if arrayNode := parentNode.FindFirstKind(artifacts.NodeInputArray); arrayNode != nil {
		return extractItemsFromChildren(builder, arrayNode, globalVariables)
	}
	if arrayNode := parentNode.FindFirstKind(artifacts.NodeOutputArray); arrayNode != nil {
		return extractItemsFromChildren(builder, arrayNode, globalVariables)
	}

	// Otherwise, it's a single item (string, glob, path, var_ref)
	singleItem := resolveArtifactItem(builder, parentNode, globalVariables)
	if singleItem != "" {
		resolvedItems = append(resolvedItems, singleItem)
	}

	return resolvedItems
}

func extractItemsFromChildren(
	builder *irBuilder,
	arrayNode *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) []string {
	var items []string
	for _, child := range arrayNode.ChildrenUnsafe() {
		resolved := resolveArtifactItem(builder, child, globalVariables)
		if resolved != "" {
			items = append(items, resolved)
		}
	}
	return items
}

func resolveArtifactItem(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) string {
	// 1. Is it a raw string?
	if strNode := node.FindFirstKind(artifacts.NodeStringLiteral); strNode != nil {
		return extractStringFromStringNode(builder, strNode, globalVariables)
	}

	// 2. Is it a variable reference?
	if varNode := node.FindFirstKind(artifacts.NodeVariableReference); varNode != nil {
		varName := extractContentFromSingleTokenNode(builder, varNode)
		if val, exists := globalVariables[varName]; exists {
			return val
		}
		emitSemanticError(builder, varNode, ERROR_UNDECLARED_VARIABLE, fmt.Sprintf("use of undeclared variable '%s' in artifacts", varName))
		return ""
	}

	// 3. Is it a PATH/GLOB function? (Stubs to be replaced by your actual path/glob evaluators)
	if pathNode := node.FindFirstKind(artifacts.NodePath); pathNode != nil {
		return "[EVALUATED_PATH]" // TODO: Implement path arg resolution
	}
	if globNode := node.FindFirstKind(artifacts.NodeGlob); globNode != nil {
		return "[EVALUATED_GLOB]" // TODO: Implement glob kwarg resolution
	}

	return ""
}

func extractBooleanNode(builder *irBuilder, node *syntaxa.SyntaxaLSTNode[artifacts.Node]) bool {
	if node == nil {
		return false
	}
	val := extractContentFromSingleTokenNode(builder, node)
	return val == "true"
}

func handleTargetWorkDir(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
	currentTarget *HelmTarget,
	state *targetParseState,
) {
	if state.workDirDeclared {
		emitSemanticError(builder, node, ERROR_DUPLICATE_WORKDIR, fmt.Sprintf("workdir for target '%s' already declared", currentTarget.name))
		return
	}
	state.workDirDeclared = true

	stringNode := node.FindFirstKind(artifacts.NodeStringLiteral)
	currentTarget.workDir = extractStringFromStringNode(builder, stringNode, globalVariables)
}

func handleTargetEnv(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
	currentTarget *HelmTarget,
	state *targetParseState,
) {
	if state.envDeclared {
		emitSemanticError(builder, node, ERROR_DUPLICATE_ENV, fmt.Sprintf("env block for target '%s' already declared", currentTarget.name))
		return
	}
	state.envDeclared = true

	keyNodes := node.FindAllKind(artifacts.NodeEnvKey)
	valueNodes := node.FindAllKind(artifacts.NodeStringLiteral)

	for i, keyNode := range keyNodes {
		keyStr := extractContentFromSingleTokenNode(builder, keyNode)

		if _, exists := currentTarget.env[keyStr]; exists {
			emitSemanticError(builder, keyNode, ERROR_DUPLICATE_ENV_KEY, fmt.Sprintf("environment variable '%s' declared multiple times", keyStr))
		} else {
			valStr := extractStringFromStringNode(builder, valueNodes[i], globalVariables)
			currentTarget.env[keyStr] = valStr
		}
	}
}

func handleTargetRun(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
	currentTarget *HelmTarget,
) {
	stringNode := node.FindFirstKind(artifacts.NodeStringLiteral)
	runText := extractStringFromStringNode(builder, stringNode, globalVariables)
	currentTarget.runs = append(currentTarget.runs, runText)
}

func handleTargetConditional(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
	currentTarget *HelmTarget,
) {
	condition := HelmCondition{}

	// 1. Identify the Parameter
	paramNode := node.FindFirstKind(artifacts.NodeConditionalParameter)
	paramName := extractContentFromSingleTokenNode(builder, paramNode)
	condition.Parameter = paramName

	// SEMANTIC VALIDATION: Does the parameter exist on the target signature?
	paramExists := false
	for _, p := range currentTarget.parameters {
		if p.name == paramName {
			paramExists = true
			break
		}
	}

	if !paramExists {
		emitSemanticError(builder, paramNode, ERROR_UNDECLARED_PARAMETER, fmt.Sprintf("condition references undeclared parameter '%s'", paramName))
	}

	// 2. Identify the Condition Type
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

	// 3. Extract the Target Value (if Equals/NotEquals)
	if condition.ConditionType == ConditionEquals || condition.ConditionType == ConditionNotEquals {
		targetValueNode := node.FindFirstKind(artifacts.NodeConditionalTarget)
		if strNode := targetValueNode.FindFirstKind(artifacts.NodeStringLiteral); strNode != nil {
			condition.TargetValue = extractStringFromStringNode(builder, strNode, globalVariables)
		} else if intNode := targetValueNode.FindFirstKind(artifacts.NodeNumber); intNode != nil {
			condition.TargetValue = extractContentFromSingleTokenNode(builder, intNode)
		}
	}

	// 4. Extract the nested Run Statements
	runNodes := node.FindAllKind(artifacts.NodeRunStatement)
	for _, runNode := range runNodes {
		stringNode := runNode.FindFirstKind(artifacts.NodeStringLiteral)
		runText := extractStringFromStringNode(builder, stringNode, globalVariables)
		condition.Runs = append(condition.Runs, runText)
	}

	currentTarget.conditionals = append(currentTarget.conditionals, condition)
}

// ---------------------------------------------------------------- Extraction

func extractStringsFromStringArrayNode(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) []string {
	stringNodes := node.FindAllKind(artifacts.NodeStringLiteral)
	out := make([]string, len(stringNodes))

	for i := 0; i < len(stringNodes); i++ {
		out[i] = extractStringFromStringNode(builder, stringNodes[i], globalVariables)
	}

	return out
}

func extractStringFromStringNode(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) string {
	signal.SignalContextPushSpan(builder.signalCtx, "String Extraction")
	defer signal.SignalContextPopSpan(builder.signalCtx)

	sb := &strings.Builder{}

	elements := node.ChildrenUnsafe()
	for _, element := range elements {
		kind := element.Kind()

		switch kind {
		case artifacts.NodeStringInterpolation:
			handleStringInterpolation(builder, sb, element, globalVariables)
		case artifacts.NodeStringDollar, artifacts.NodeStringText:
			content := extractContentFromSingleTokenNode(builder, element)
			sb.WriteString(content)
		case artifacts.NodeStringEscape:
			handleStringEscape(builder, sb, element)
		default:
			panic(fmt.Errorf("interpreter error: unhandled child kind '%v'", kind))
		}
	}

	return sb.String()
}

func handleStringInterpolation(
	builder *irBuilder,
	sb *strings.Builder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	globalVariables map[string]string,
) {
	targetVariableNode := node.FindDirectChildKind(artifacts.NodeInterpolatedVariable)
	targetVariableIdentifier := extractContentFromSingleTokenNode(builder, targetVariableNode)

	if value, exists := globalVariables[targetVariableIdentifier]; exists {
		sb.WriteString(value)
	} else {
		emitSemanticError(builder, targetVariableNode, ERROR_UNDECLARED_VARIABLE, fmt.Sprintf("use of undeclared variable '%s'", targetVariableIdentifier))
	}
}

func extractContentFromSingleTokenNode(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
) string {
	tks := node.Tokens()
	amnt := len(tks)

	if amnt != 1 {
		panic(fmt.Errorf("interpreter error: node does not have exactly 1 tokens but got %d\ndiagnostic:\n%s", amnt,
			node.DebugDump(syntaxa.LSTDebugFormatter[artifacts.Node]{
				FormatKind: artifacts.Node.String,
			}),
		))
	}

	return string(tks[0].Raw)
}

func handleStringEscape(
	builder *irBuilder,
	sb *strings.Builder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
) {
	raw := string(node.Tokens()[0].Raw)

	if len(raw) < 2 {
		return
	}

	switch raw[1] {
	case '"', '\\':
		sb.WriteByte(raw[1])
	case 'n':
		sb.WriteByte('\n')
	case 'r':
		sb.WriteByte('\r')
	case 't':
		sb.WriteByte('\t')
	case 'u', 'U':
		decodeAndAppendUnicode(builder, sb, node, raw)
	default:
		sb.WriteString(raw)
	}
}

func extractParameterFromNode(builder *irBuilder, node *syntaxa.SyntaxaLSTNode[artifacts.Node]) HelmTargetParameter {
	parameter := HelmTargetParameter{}

	name := extractContentFromSingleTokenNode(builder, node.FindDirectChildKind(artifacts.NodeTargetParameterIdentifier))
	optional := false

	if node.FindDirectChildKind(artifacts.NodeTargetParameterOptional) != nil {
		optional = true
	}

	parameter.name = name
	parameter.optional = optional

	return parameter
}

// ---------------------------------------------------------------- UTILITIES

func emitSemanticError(builder *irBuilder, node *syntaxa.SyntaxaLSTNode[artifacts.Node], errorID string, message string) {
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

func decodeAndAppendUnicode(
	builder *irBuilder,
	sb *strings.Builder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	raw string,
) {
	hexString := raw[2:]

	codePoint, err := strconv.ParseInt(hexString, 16, 32)
	if err != nil {
		emitSemanticError(
			builder,
			node,
			"ERR_UNICODE_DECODE",
			fmt.Sprintf("failed to decode unicode escape sequence: %v", err),
		)
		return
	}

	sb.WriteRune(rune(codePoint))
}
