package interpreter

import (
	"fmt"
	"helm/internal/targetexecutor"
)

// HelmInterpreterExportExecutionGraph resolves the full execution graph for entryTarget
// without running any target (graph introspection / dry-run export).
func HelmInterpreterExportExecutionGraph(
	result HelmInterpreterInterpretationResult,
	entryTarget string,
	invocations map[string]targetexecutor.TargetInvocation,
) (*targetexecutor.TargetExecutionGraphExport, error) {
	if result.Error != nil {
		return nil, result.Error
	}
	if !result.BuiltIR.Succeeded {
		return nil, fmt.Errorf("interpretation aborted due to semantic errors")
	}
	return targetexecutor.TargetExecutorExportExecutionGraph(
		result.BuiltIR,
		entryTarget,
		invocations,
	)
}

// HelmInterpreterExportExecutionGraphJSON returns indented JSON for the resolved graph.
func HelmInterpreterExportExecutionGraphJSON(
	result HelmInterpreterInterpretationResult,
	entryTarget string,
	invocations map[string]targetexecutor.TargetInvocation,
) ([]byte, error) {
	return targetexecutor.TargetExecutorExportExecutionGraphJSON(
		result.BuiltIR,
		entryTarget,
		invocations,
	)
}

// HelmInterpreterExportExecutionGraphBundle resolves one graph per entry target without running.
func HelmInterpreterExportExecutionGraphBundle(
	result HelmInterpreterInterpretationResult,
	entryTargets []string,
	invocations map[string]targetexecutor.TargetInvocation,
) (*targetexecutor.TargetExecutionGraphExportBundle, error) {
	if result.Error != nil {
		return nil, result.Error
	}
	if !result.BuiltIR.Succeeded {
		return nil, fmt.Errorf("interpretation aborted due to semantic errors")
	}
	return targetexecutor.TargetExecutorExportExecutionGraphBundle(
		result.BuiltIR,
		entryTargets,
		invocations,
	)
}

// HelmInterpreterExportExecutionGraphBundleJSON returns indented JSON for multiple graphs.
func HelmInterpreterExportExecutionGraphBundleJSON(
	result HelmInterpreterInterpretationResult,
	entryTargets []string,
	invocations map[string]targetexecutor.TargetInvocation,
) ([]byte, error) {
	return targetexecutor.TargetExecutorExportExecutionGraphBundleJSON(
		result.BuiltIR,
		entryTargets,
		invocations,
	)
}
