package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"syntaxa"
)

func handleTargetLetBindings(
	builder *irBuilder,
	contentNodes []*syntaxa.SyntaxaLSTNode[artifacts.Node],
	baseScope resolveScope,
	currentTarget *HelmTarget,
) resolveScope {
	scope := baseScope

	for _, contentNode := range contentNodes {
		if contentNode.Kind() != artifacts.NodeLetBinding {
			continue
		}

		nameNode := contentNode.FindDirectChildKind(artifacts.NodeLetBindingName)
		name := extractContentFromSingleTokenNode(builder, nameNode)

		if resolveScopeHasParameter(scope, name) {
			emitSemanticError(
				builder,
				nameNode,
				ERROR_LET_SHADOWS_BINDING,
				fmt.Sprintf("let cannot shadow parameter '%s'", name),
			)
			continue
		}
		if _, isGlobal := scope.globals[name]; isGlobal {
			emitSemanticError(
				builder,
				nameNode,
				ERROR_LET_SHADOWS_BINDING,
				fmt.Sprintf("let cannot shadow global '%s'", name),
			)
			continue
		}
		if resolveScopeHasLetBinding(scope, name) {
			emitSemanticError(
				builder,
				nameNode,
				ERROR_DUPLICATE_LET,
				fmt.Sprintf("let binding '%s' already declared", name),
			)
			continue
		}

		expr, ok := extractPathExprFromNode(builder, contentNode, scope)
		if !ok {
			continue
		}

		currentTarget.LetBindings = append(currentTarget.LetBindings, HelmPathBinding{
			Name: name,
			Expr: expr,
		})
		scope.letBindings = append(scope.letBindings, name)
	}

	return scope
}
