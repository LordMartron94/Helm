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
	globals    map[string]string
	parameters []HelmTargetParameter
}

func resolveScopeForGlobals(globals map[string]string) resolveScope {
	return resolveScope{globals: globals}
}

func resolveScopeForTarget(globals map[string]string, parameters []HelmTargetParameter) resolveScope {
	return resolveScope{globals: globals, parameters: parameters}
}

func resolveScopeHasParameter(scope resolveScope, name string) bool {
	for _, p := range scope.parameters {
		if p.name == name {
			return true
		}
	}
	return false
}

func resolveInterpolation(scope resolveScope, ident string) (text string, ok bool) {
	if value, exists := scope.globals[ident]; exists {
		return value, true
	}
	if resolveScopeHasParameter(scope, ident) {
		return "${" + ident + "}", true
	}
	return "", false
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

	if text, ok := resolveInterpolation(scope, targetVariableIdentifier); ok {
		sb.WriteString(text)
	} else {
		emitSemanticError(
			builder,
			targetVariableNode,
			ERROR_UNDECLARED_VARIABLE,
			fmt.Sprintf("use of undeclared variable '%s'", targetVariableIdentifier),
		)
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

	parameter.name = name
	parameter.optional = optional

	return parameter
}
