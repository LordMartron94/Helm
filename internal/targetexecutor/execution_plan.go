package targetexecutor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"helm/internal/ir"
	"strings"
	"structarch"
)

const targetExecutorParametricInstanceSeparator = "#"

// TargetExecutionPlan is the expanded DAG used for one helm run: parametric dependency
// edges become distinct nodes (canonical#instance) so the same target definition can
// run multiple times with different params; identical bound params share one instance.
type TargetExecutionPlan struct {
	Phases          [][]string
	NodeInvocations map[string]TargetInvocation
	// Outgoing maps each canonical target to execution node IDs for its depends_on edges.
	Outgoing map[string][]string
}

func TargetExecutorBuildExecutionPlan(
	builtIR ir.HelmIR,
	entryTarget string,
	callerInvocations map[string]TargetInvocation,
) (*TargetExecutionPlan, error) {
	canonical, ok := ir.IRResolveTargetName(builtIR.Targets, entryTarget)
	if !ok {
		return nil, fmt.Errorf("target '%s' does not exist in IR", entryTarget)
	}

	graph := make(map[string][]string)
	nodeInvocations := make(map[string]TargetInvocation)

	if err := targetExecutorInsertExecutionNode(
		builtIR,
		canonical,
		callerInvocations,
		graph,
		nodeInvocations,
		make(map[string]struct{}),
	); err != nil {
		return nil, err
	}

	phases, err := structarch.STRUCTARCH_DAG_Resolve(graph)
	if err != nil {
		return nil, err
	}

	return &TargetExecutionPlan{
		Phases:          phases,
		NodeInvocations: nodeInvocations,
		Outgoing:        graph,
	}, nil
}

func TargetExecutorExecutionNodeCanonical(nodeID string) string {
	separator := strings.Index(nodeID, targetExecutorParametricInstanceSeparator)
	if separator < 0 {
		return nodeID
	}
	return nodeID[:separator]
}

func TargetExecutorExecutionNodeInstanceKey(nodeID string) string {
	separator := strings.Index(nodeID, targetExecutorParametricInstanceSeparator)
	if separator < 0 {
		return ""
	}
	return "params/" + nodeID[separator+1:]
}

func targetExecutorParametricInstanceID(
	canonical string,
	bound map[string]ir.HelmParameterValue,
) string {
	fingerprint := targetExecutorParameterMapFingerprint(bound)
	sum := sha256.Sum256([]byte(fingerprint))
	return canonical + targetExecutorParametricInstanceSeparator + hex.EncodeToString(sum[:8])
}

func targetExecutorInsertExecutionNode(
	builtIR ir.HelmIR,
	dependentName string,
	callerInvocations map[string]TargetInvocation,
	graph map[string][]string,
	nodeInvocations map[string]TargetInvocation,
	visiting map[string]struct{},
) error {
	if visiting == nil {
		visiting = make(map[string]struct{})
	}
	if _, exists := graph[dependentName]; exists {
		return nil
	}

	if _, onStack := visiting[dependentName]; onStack {
		return fmt.Errorf("cyclic dependency graph involving target '%s'", dependentName)
	}
	visiting[dependentName] = struct{}{}
	defer delete(visiting, dependentName)

	target, ok := builtIR.Targets[dependentName]
	if !ok {
		return fmt.Errorf("target '%s' does not exist in IR", dependentName)
	}

	inv := targetExecutorInvocationForInsert(dependentName, callerInvocations, nodeInvocations)

	parentResolved, err := targetExecutorResolvedParamsForTarget(
		builtIR,
		targetExecutorClosureFromGraph(graph),
		dependentName,
		callerInvocations,
		nil,
	)
	if err != nil {
		return fmt.Errorf("target '%s': %w", dependentName, err)
	}

	effectiveDeps, err := targetExecutorEffectiveDependsOn(
		builtIR,
		target,
		inv,
		parentResolved,
	)
	if err != nil {
		return fmt.Errorf("target '%s': %w", dependentName, err)
	}

	depNodes, err := targetExecutorRegisterDependencyNodes(
		builtIR,
		dependentName,
		effectiveDeps,
		nodeInvocations,
	)
	if err != nil {
		return err
	}

	graph[dependentName] = depNodes

	for _, execNode := range depNodes {
		canonical := TargetExecutorExecutionNodeCanonical(execNode)
		if execNode == canonical {
			if err := targetExecutorInsertExecutionNode(
				builtIR,
				canonical,
				callerInvocations,
				graph,
				nodeInvocations,
				visiting,
			); err != nil {
				return err
			}
			continue
		}

		if err := targetExecutorInsertParametricInstanceNode(
			builtIR,
			execNode,
			nodeInvocations[execNode],
			graph,
			nodeInvocations,
			visiting,
		); err != nil {
			return err
		}
	}

	return nil
}

func targetExecutorClosureFromGraph(graph map[string][]string) map[string]struct{} {
	closure := make(map[string]struct{}, len(graph))
	for nodeID := range graph {
		closure[nodeID] = struct{}{}
		closure[TargetExecutorExecutionNodeCanonical(nodeID)] = struct{}{}
	}
	return closure
}

func targetExecutorInsertParametricInstanceNode(
	builtIR ir.HelmIR,
	execNode string,
	inv TargetInvocation,
	graph map[string][]string,
	nodeInvocations map[string]TargetInvocation,
	visiting map[string]struct{},
) error {
	if _, exists := graph[execNode]; exists {
		return nil
	}

	canonical := TargetExecutorExecutionNodeCanonical(execNode)
	target, ok := builtIR.Targets[canonical]
	if !ok {
		return fmt.Errorf("target '%s' does not exist in IR", canonical)
	}

	parentResolved, err := TargetExecutorResolveInvocationParameters(
		builtIR.SourceDirectory,
		ir.IRGlobalsForRootManifest(builtIR),
		TargetExecutorParametersForTarget(target, inv),
	)
	if err != nil {
		return fmt.Errorf("parametric instance '%s': %w", execNode, err)
	}

	effectiveDeps, err := targetExecutorEffectiveDependsOn(
		builtIR,
		target,
		inv,
		parentResolved,
	)
	if err != nil {
		return fmt.Errorf("parametric instance '%s': %w", execNode, err)
	}

	depNodes, err := targetExecutorRegisterDependencyNodes(
		builtIR,
		canonical,
		effectiveDeps,
		nodeInvocations,
	)
	if err != nil {
		return err
	}

	graph[execNode] = depNodes

	for _, childExecNode := range depNodes {
		childCanonical := TargetExecutorExecutionNodeCanonical(childExecNode)
		if childExecNode == childCanonical {
			if err := targetExecutorInsertExecutionNode(
				builtIR,
				childCanonical,
				nil,
				graph,
				nodeInvocations,
				visiting,
			); err != nil {
				return err
			}
			continue
		}

		if err := targetExecutorInsertParametricInstanceNode(
			builtIR,
			childExecNode,
			nodeInvocations[childExecNode],
			graph,
			nodeInvocations,
			visiting,
		); err != nil {
			return err
		}
	}

	return nil
}
