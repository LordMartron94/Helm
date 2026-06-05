package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"syntaxa"
)

func parseAdapterPhases(
	builder *irBuilder,
	body *syntaxa.SyntaxaLSTNode[artifacts.Node],
	decl *HelmAdapterDecl,
	baseScope resolveScope,
) bool {
	if body == nil {
		return false
	}

	phaseNodes := body.FindAllKind(artifacts.NodeAdapterPhase)
	if len(phaseNodes) == 0 {
		return false
	}

	seenNames := make(map[string]struct{}, len(phaseNodes))
	for _, phaseNode := range phaseNodes {
		nameNode := phaseNode.FindDirectChildKind(artifacts.NodeAdapterPhaseName)
		phaseName := extractContentFromSingleTokenNode(builder, nameNode)
		if _, exists := seenNames[phaseName]; exists {
			emitSemanticError(
				builder,
				nameNode,
				ERROR_DUPLICATE_TARGET,
				fmt.Sprintf("adapter '%s' phase '%s' already declared", decl.Name, phaseName),
			)
			continue
		}
		seenNames[phaseName] = struct{}{}

		phase := HelmAdapterPhase{
			Name: phaseName,
			Env:  make(map[string]HelmStringListExpr),
		}
		phaseBody := phaseNode.FindDirectChildKind(artifacts.NodeAdapterPhaseBody)
		parseAdapterPhaseBody(builder, phaseBody, decl, baseScope, &phase)
		decl.Phases = append(decl.Phases, phase)
	}

	adapterValidatePhases(builder, decl)
	return true
}

func parseAdapterPhaseBody(
	builder *irBuilder,
	body *syntaxa.SyntaxaLSTNode[artifacts.Node],
	decl *HelmAdapterDecl,
	baseScope resolveScope,
	phase *HelmAdapterPhase,
) {
	if body == nil {
		return
	}

	children := body.ChildrenUnsafe()
	for _, child := range children {
		if child.Kind() == artifacts.NodeMatrixBlock {
			parseAdapterPhaseMatrix(builder, child, decl, baseScope, phase)
		}
	}

	matrixVariable := ""
	if phase.Matrix != nil {
		matrixVariable = phase.Matrix.VariableName
	}
	scope := resolveScopeForAdapter(baseScope, decl.Parameters, matrixVariable)
	matrixScope := resolveScopeForAdapter(baseScope, decl.Parameters, matrixVariable)

	inMatrixSection := phase.Matrix != nil
	matrixLegOutputsDeclared := false

	for _, child := range children {
		switch child.Kind() {
		case artifacts.NodeMatrixBlock:
			continue
		case artifacts.NodeAdapterPhaseDepends:
			phase.DependsOn = append(phase.DependsOn, extractAdapterPhaseDepends(builder, child)...)
		case artifacts.NodeAdapterOutputs:
			if inMatrixSection && !matrixLegOutputsDeclared {
				phase.MatrixLegOutputs = extractArtifactItemsFromPathArrayRoot(builder, child, matrixScope)
				matrixLegOutputsDeclared = true
				continue
			}
			phase.Outputs = extractArtifactItemsFromPathArrayRoot(builder, child, scope)
		case artifacts.NodeAdapterEnv:
			parseAdapterPhaseEnv(builder, child, scope, phase)
			inMatrixSection = false
		case artifacts.NodeRunStatement:
			runCommand := extractRunCommandFromNode(builder, child, scope)
			if inMatrixSection {
				runCommand = extractRunCommandFromNode(builder, child, matrixScope)
				phase.MatrixRuns = append(phase.MatrixRuns, runCommand)
				continue
			}
			phase.Runs = append(phase.Runs, runCommand)
		default:
			emitSemanticError(
				builder,
				child,
				ERROR_INVALID_VARIABLE_VALUE,
				fmt.Sprintf("adapter '%s' phase '%s' contains an unsupported statement", decl.Name, phase.Name),
			)
		}
	}

	if phase.Matrix != nil && len(phase.MatrixRuns) == 0 {
		emitSemanticError(
			builder,
			body,
			ERROR_EMPTY_MATRIX,
			fmt.Sprintf("adapter '%s' phase '%s' matrix must declare at least one run command", decl.Name, phase.Name),
		)
	}
	if len(phase.Runs) == 0 && (phase.Matrix == nil || len(phase.MatrixRuns) == 0) {
		emitSemanticError(
			builder,
			body,
			ERROR_INVALID_VARIABLE_VALUE,
			fmt.Sprintf("adapter '%s' phase '%s' must declare at least one run command", decl.Name, phase.Name),
		)
	}
}

func parseAdapterPhaseMatrix(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	decl *HelmAdapterDecl,
	baseScope resolveScope,
	phase *HelmAdapterPhase,
) {
	if phase.Matrix != nil {
		emitSemanticError(
			builder,
			node,
			ERROR_DUPLICATE_MATRIX,
			fmt.Sprintf("adapter '%s' phase '%s' already has a matrix block", decl.Name, phase.Name),
		)
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
			fmt.Sprintf("adapter '%s' phase '%s' matrix must have at least one value", decl.Name, phase.Name),
		)
		return
	}

	phase.Matrix = &HelmMatrix{
		VariableName: variableName,
		Values:       values,
	}
}

func extractAdapterPhaseDepends(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
) []string {
	arrayNode := node.FindFirstKind(artifacts.NodeAdapterPhaseDependsArray)
	if arrayNode == nil {
		return nil
	}
	var names []string
	for _, child := range arrayNode.ChildrenUnsafe() {
		if child.Kind() == artifacts.NodeAdapterPhaseDependsName {
			idNode := child.FindDirectChildKind(artifacts.NodeAdapterPhaseDependsIdentifier)
			if idNode != nil {
				names = append(names, extractContentFromSingleTokenNode(builder, idNode))
			}
		}
	}
	return names
}

func parseAdapterPhaseEnv(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	phase *HelmAdapterPhase,
) {
	decl := &HelmAdapterDecl{Name: "", Env: phase.Env}
	handleAdapterEnv(builder, node, scope, decl)
	phase.Env = decl.Env
}

func adapterValidatePhases(builder *irBuilder, decl *HelmAdapterDecl) {
	byName := make(map[string]int, len(decl.Phases))
	for index, phase := range decl.Phases {
		byName[phase.Name] = index
	}

	for _, phase := range decl.Phases {
		for _, dep := range phase.DependsOn {
			if _, ok := byName[dep]; !ok {
				emitSemanticError(
					builder,
					nil,
					ERROR_UNDECLARED_TARGET,
					fmt.Sprintf("adapter '%s' phase '%s' depends on unknown phase '%s'", decl.Name, phase.Name, dep),
				)
			}
		}
	}

	if _, err := AdapterPhaseTopoOrder(decl.Phases); err != nil {
		emitSemanticError(builder, nil, ERROR_INVALID_VARIABLE_VALUE, err.Error())
	}
}

// AdapterPhaseTopoOrder returns phases in dependency order.
func AdapterPhaseTopoOrder(phases []HelmAdapterPhase) ([]HelmAdapterPhase, error) {
	byName := make(map[string]HelmAdapterPhase, len(phases))
	indegree := make(map[string]int, len(phases))
	dependents := make(map[string][]string, len(phases))

	for _, phase := range phases {
		byName[phase.Name] = phase
		if _, exists := indegree[phase.Name]; !exists {
			indegree[phase.Name] = 0
		}
		for _, dep := range phase.DependsOn {
			if _, ok := byName[dep]; !ok {
				return nil, fmt.Errorf("adapter phase %q depends on unknown phase %q", phase.Name, dep)
			}
			indegree[phase.Name]++
			dependents[dep] = append(dependents[dep], phase.Name)
		}
	}

	queue := make([]string, 0, len(phases))
	for name, degree := range indegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	ordered := make([]HelmAdapterPhase, 0, len(phases))
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		ordered = append(ordered, byName[name])
		for _, child := range dependents[name] {
			indegree[child]--
			if indegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}

	if len(ordered) != len(phases) {
		return nil, fmt.Errorf("adapter phase dependency cycle detected")
	}
	return ordered, nil
}
