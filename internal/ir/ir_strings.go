package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"signal"
	"strconv"
	"strings"
	"syntaxa"
)

type resolveScope struct {
	globals           map[string]HelmGlobalVariable
	parameters        []HelmTargetParameter
	matrixVariable    string
	letBindings       []string
	permissiveParams  bool
	permissiveGlobals bool
	// deferGlobalInterpolation keeps ${GLOBAL} placeholders in adapter templates so
	// configuration profiles can override workspace globals at entity expansion time.
	deferGlobalInterpolation bool
}

func resolveScopeForGlobals(globals map[string]HelmGlobalVariable) resolveScope {
	return resolveScope{globals: globals}
}

func resolveScopeForTarget(globals map[string]HelmGlobalVariable, parameters []HelmTargetParameter) resolveScope {
	return resolveScope{globals: globals, parameters: parameters}
}

func resolveScopeForTargetWithMatrix(
	globals map[string]HelmGlobalVariable,
	parameters []HelmTargetParameter,
	matrixVariable string,
) resolveScope {
	return resolveScope{
		globals:        globals,
		parameters:     parameters,
		matrixVariable: matrixVariable,
	}
}

// resolveScopeForAdapter builds a scope for adapter argv templates (declared parameters + engine params).
func resolveScopeForAdapter(
	base resolveScope,
	parameters []HelmTargetParameter,
	matrixVariable string,
) resolveScope {
	adapterParams := append([]HelmTargetParameter(nil), parameters...)
	seen := make(map[string]struct{}, len(adapterParams)+2)
	for _, parameter := range adapterParams {
		seen[parameter.Name] = struct{}{}
	}
	if _, ok := seen["MATRIX_OUTPUTS"]; !ok {
		adapterParams = append(adapterParams, HelmTargetParameter{Name: "MATRIX_OUTPUTS"})
	}
	for name, variable := range base.globals {
		if variable.Kind == HelmGlobalVarArtifactArray {
			if _, ok := seen[name]; !ok {
				adapterParams = append(adapterParams, HelmTargetParameter{Name: name})
			}
		}
	}
	scope := resolveScopeForTargetWithMatrix(base.globals, adapterParams, matrixVariable)
	scope.permissiveParams = true
	scope.permissiveGlobals = true
	scope.deferGlobalInterpolation = true
	return scope
}

func resolveScopeAllowsParamRef(scope resolveScope, name string) bool {
	if resolveScopeHasParameter(scope, name) {
		return true
	}
	if resolveScopeHasLetBinding(scope, name) {
		return true
	}
	if globalVariableIsArtifactArray(scope, name) {
		return true
	}
	return scope.permissiveParams
}

func resolveScopeMatrixVariable(scope resolveScope) string {
	return scope.matrixVariable
}

func resolveScopeHasLetBinding(scope resolveScope, name string) bool {
	for _, binding := range scope.letBindings {
		if binding == name {
			return true
		}
	}
	return false
}

func resolveScopeHasParameter(scope resolveScope, name string) bool {
	for _, p := range scope.parameters {
		if p.Name == name {
			return true
		}
	}
	return false
}

func resolveScopeParameterPlaceholder(scope resolveScope, name string) (string, bool) {
	if resolveScopeHasParameter(scope, name) {
		return "${" + name + "}", true
	}
	return "", false
}

// resolveScopeVariableAsLiteral resolves a bare variable reference for scalar contexts
// (path segments, artifact string literals). Globals are expanded at IR build time;
// target and matrix variables become runtime placeholders.
func resolveScopeVariableAsLiteral(scope resolveScope, name string) (string, bool) {
	if text, ok := resolveGlobalString(scope, name); ok {
		if scope.deferGlobalInterpolation {
			return "${" + name + "}", true
		}
		return text, true
	}
	if placeholder, ok := resolveScopeParameterPlaceholder(scope, name); ok {
		return placeholder, true
	}
	if scope.matrixVariable != "" && name == scope.matrixVariable {
		return "${" + name + "}", true
	}
	if scope.permissiveGlobals {
		return "${" + name + "}", true
	}
	return "", false
}

func resolveInterpolation(scope resolveScope, ident string) (text string, ok bool) {
	if value, exists := scope.globals[ident]; exists {
		if value.Kind == HelmGlobalVarString {
			return value.StringValue, true
		}
		return "", false
	}
	if resolveScopeHasParameter(scope, ident) {
		return "${" + ident + "}", true
	}
	if scope.matrixVariable != "" && ident == scope.matrixVariable {
		return "${" + ident + "}", true
	}
	return "", false
}

func findStringContentNode(
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
) *syntaxa.SyntaxaLSTNode[artifacts.Node] {
	if node == nil {
		return nil
	}
	if stringNode := node.FindFirstKind(artifacts.NodeStringLiteral); stringNode != nil {
		return stringNode
	}
	return node.FindFirstKind(artifacts.NodeMultilineString)
}

func extractStringsFromStringArrayNode(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) []string {
	stringNodes := node.FindAllKind(artifacts.NodeStringLiteral)
	out := make([]string, len(stringNodes))

	for i := 0; i < len(stringNodes); i++ {
		out[i] = extractStringFromStringNode(builder, stringNodes[i], scope)
	}

	return out
}

func extractStringFromStringNode(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) string {
	signal.SignalContextPushSpan(builder.signalCtx, "String Extraction")
	defer signal.SignalContextPopSpan(builder.signalCtx)

	sb := &strings.Builder{}

	elements := node.ChildrenUnsafe()
	for _, element := range elements {
		kind := element.Kind()

		switch kind {
		case artifacts.NodeStringInterpolation:
			handleStringInterpolation(builder, sb, element, scope)
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
	scope resolveScope,
) {
	targetVariableNode := node.FindDirectChildKind(artifacts.NodeInterpolatedVariable)
	targetVariableIdentifier := extractContentFromSingleTokenNode(builder, targetVariableNode)

	if variable, exists := scope.globals[targetVariableIdentifier]; exists {
		if variable.Kind == HelmGlobalVarString {
			if scope.deferGlobalInterpolation {
				sb.WriteString("${" + targetVariableIdentifier + "}")
				return
			}
			sb.WriteString(variable.StringValue)
			return
		}
		if variable.Kind == HelmGlobalVarArtifactArray {
			sb.WriteString("${" + targetVariableIdentifier + "}")
			return
		}
	}
	if resolveScopeHasParameter(scope, targetVariableIdentifier) {
		sb.WriteString("${" + targetVariableIdentifier + "}")
		return
	}
	if scope.matrixVariable != "" && targetVariableIdentifier == scope.matrixVariable {
		sb.WriteString("${" + targetVariableIdentifier + "}")
		return
	}
	if scope.permissiveGlobals {
		sb.WriteString("${" + targetVariableIdentifier + "}")
		return
	}
	emitSemanticError(
		builder,
		targetVariableNode,
		ERROR_UNDECLARED_VARIABLE,
		fmt.Sprintf("use of undeclared variable '%s'", targetVariableIdentifier),
	)
}

func extractContentFromSingleTokenNode(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
) string {
	text, ok := tryExtractTokenTextFromNode(node)
	if !ok {
		panic(fmt.Errorf(
			"interpreter error: node does not contain exactly one token\ndiagnostic:\n%s",
			node.DebugDump(syntaxa.LSTDebugFormatter[artifacts.Node]{
				FormatKind: artifacts.Node.String,
			}),
		))
	}
	return text
}

func tryExtractTokenTextFromNode(
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
) (string, bool) {
	if node == nil {
		return "", false
	}

	tks := node.Tokens()
	if len(tks) == 1 {
		return string(tks[0].Raw), true
	}

	for _, child := range node.ChildrenUnsafe() {
		if text, ok := tryExtractTokenTextFromNode(child); ok {
			return text, true
		}
	}

	return "", false
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

func extractParameterFromNode(builder *irBuilder, node *syntaxa.SyntaxaLSTNode[artifacts.Node]) HelmTargetParameter {
	parameter := HelmTargetParameter{}

	name := extractContentFromSingleTokenNode(builder, node.FindDirectChildKind(artifacts.NodeTargetParameterIdentifier))
	optional := false

	if node.FindDirectChildKind(artifacts.NodeTargetParameterOptional) != nil {
		optional = true
	}

	parameter.Name = name
	parameter.Optional = optional

	return parameter
}
