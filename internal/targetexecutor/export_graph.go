package targetexecutor

import (
	"encoding/json"
	"fmt"
	"helm/internal/ir"
	"path/filepath"
	"sort"
)

// TargetExecutionGraphExport is the resolved execution graph for introspection (no runs executed).
type TargetExecutionGraphExport struct {
	EntryTarget     string                            `json:"entry_target"`
	SourceDirectory string                            `json:"source_directory"`
	Phases          [][]string                        `json:"phases"`
	Targets         map[string]TargetGraphExportEntry `json:"targets"`
}

// TargetExecutionGraphExportBundle holds one resolved graph per entry target.
type TargetExecutionGraphExportBundle struct {
	SourceDirectory string                                 `json:"source_directory"`
	Graphs          map[string]*TargetExecutionGraphExport `json:"graphs"`
}

// TargetGraphExportEntry describes one execution node after parameter and template expansion.
type TargetGraphExportEntry struct {
	CanonicalTarget string                 `json:"canonical_target"`
	Directory       string                 `json:"directory"`
	RunCommands     []string               `json:"run_commands"`
	RunArgvs        [][]string             `json:"run_argvs,omitempty"`
	DependsOn       []string               `json:"depends_on,omitempty"`
	MatrixInstance  string                 `json:"matrix_instance,omitempty"`
	Parameters      map[string]interface{} `json:"parameters,omitempty"`
}

// TargetExecutorExportExecutionGraph builds the full resolved graph for entryTarget without executing it.
func TargetExecutorExportExecutionGraph(
	builtIR ir.HelmIR,
	entryTarget string,
	callerInvocations map[string]TargetInvocation,
) (*TargetExecutionGraphExport, error) {
	canonicalEntry, ok := ir.IRResolveTargetName(builtIR.Targets, entryTarget)
	if !ok {
		return nil, fmt.Errorf("target '%s' does not exist in IR", entryTarget)
	}

	plan, err := TargetExecutorBuildExecutionPlan(builtIR, canonicalEntry, callerInvocations)
	if err != nil {
		return nil, err
	}

	export := &TargetExecutionGraphExport{
		EntryTarget:     canonicalEntry,
		SourceDirectory: builtIR.SourceDirectory,
		Phases:          plan.Phases,
		Targets:         make(map[string]TargetGraphExportEntry),
	}

	nodeNames := targetExecutorOrderedPlanNodes(plan)
	for _, nodeName := range nodeNames {
		if err := targetExecutorExportGraphNode(
			builtIR,
			plan,
			nodeName,
			callerInvocations,
			export,
		); err != nil {
			return nil, err
		}
	}

	return export, nil
}

// TargetExecutorExportExecutionGraphJSON marshals the resolved graph with stable key ordering.
func TargetExecutorExportExecutionGraphJSON(
	builtIR ir.HelmIR,
	entryTarget string,
	callerInvocations map[string]TargetInvocation,
) ([]byte, error) {
	export, err := TargetExecutorExportExecutionGraph(builtIR, entryTarget, callerInvocations)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(export, "", "  ")
}

// TargetExecutorExportExecutionGraphBundle resolves one graph per entry target without executing.
func TargetExecutorExportExecutionGraphBundle(
	builtIR ir.HelmIR,
	entryTargets []string,
	callerInvocations map[string]TargetInvocation,
) (*TargetExecutionGraphExportBundle, error) {
	if len(entryTargets) == 0 {
		return nil, fmt.Errorf("at least one entry target is required")
	}

	bundle := &TargetExecutionGraphExportBundle{
		SourceDirectory: builtIR.SourceDirectory,
		Graphs:          make(map[string]*TargetExecutionGraphExport, len(entryTargets)),
	}

	for _, entryTarget := range entryTargets {
		export, err := TargetExecutorExportExecutionGraph(builtIR, entryTarget, callerInvocations)
		if err != nil {
			return nil, fmt.Errorf("entry target %q: %w", entryTarget, err)
		}
		bundle.Graphs[export.EntryTarget] = export
	}

	return bundle, nil
}

// TargetExecutorExportExecutionGraphBundleJSON marshals multiple resolved graphs.
func TargetExecutorExportExecutionGraphBundleJSON(
	builtIR ir.HelmIR,
	entryTargets []string,
	callerInvocations map[string]TargetInvocation,
) ([]byte, error) {
	bundle, err := TargetExecutorExportExecutionGraphBundle(builtIR, entryTargets, callerInvocations)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(bundle, "", "  ")
}

func targetExecutorOrderedPlanNodes(plan *TargetExecutionPlan) []string {
	seen := make(map[string]struct{}, len(plan.Outgoing))
	var order []string
	for _, phase := range plan.Phases {
		for _, name := range phase {
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			order = append(order, name)
		}
	}
	for name := range plan.Outgoing {
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		order = append(order, name)
	}
	sort.Strings(order)
	return order
}

func targetExecutorExportGraphNode(
	builtIR ir.HelmIR,
	plan *TargetExecutionPlan,
	nodeName string,
	callerInvocations map[string]TargetInvocation,
	export *TargetExecutionGraphExport,
) error {
	canonical := TargetExecutorExecutionNodeCanonical(nodeName)
	target, ok := builtIR.Targets[canonical]
	if !ok {
		return fmt.Errorf("target '%s' does not exist in IR", canonical)
	}

	inv := targetExecutorInvocationForNode(plan, nodeName, callerInvocations)
	paramValues := TargetExecutorParametersForTarget(target, inv)
	resolved, err := TargetExecutorResolveInvocationParameters(
		builtIR.SourceDirectory,
		builtIR.GlobalVariables,
		paramValues,
	)
	if err != nil {
		return fmt.Errorf("node '%s': %w", nodeName, err)
	}

	baseInterpCtx, err := TargetExecutorInterpolationGlobals(
		builtIR.SourceDirectory,
		builtIR.GlobalVariables,
		resolved,
	)
	if err != nil {
		return fmt.Errorf("node '%s': %w", nodeName, err)
	}
	interpCtx, err := TargetExecutorEvaluateLetBindings(builtIR.SourceDirectory, target, baseInterpCtx)
	if err != nil {
		return fmt.Errorf("node '%s': %w", nodeName, err)
	}

	instances, err := TargetExecutorMatrixInstances(
		builtIR.SourceDirectory,
		target,
		paramValues,
		builtIR.GlobalVariables,
		interpCtx,
	)
	if err != nil {
		return fmt.Errorf("node '%s': %w", nodeName, err)
	}

	dependsOn := plan.Outgoing[nodeName]
	if dependsOn == nil {
		dependsOn = []string{}
	}

	for _, instance := range instances {
		effectiveParamValues := targetExecutorEffectiveParameterValues(target, inv, instance.Bindings)
		effectiveResolved, resolveErr := TargetExecutorResolveInvocationParameters(
			builtIR.SourceDirectory,
			builtIR.GlobalVariables,
			effectiveParamValues,
		)
		if resolveErr != nil {
			return fmt.Errorf("node '%s': %w", nodeName, resolveErr)
		}

		instanceInterpCtx, interpErr := TargetExecutorInterpolationGlobals(
			builtIR.SourceDirectory,
			builtIR.GlobalVariables,
			effectiveResolved,
		)
		if interpErr != nil {
			return fmt.Errorf("node '%s': %w", nodeName, interpErr)
		}
		instanceInterpCtx, interpErr = TargetExecutorEvaluateLetBindings(
			builtIR.SourceDirectory,
			target,
			instanceInterpCtx,
		)
		if interpErr != nil {
			return fmt.Errorf("node '%s': %w", nodeName, interpErr)
		}

		workDir, steps, resolveErr := targetExecutorResolveTargetRunsFromResolved(
			builtIR.SourceDirectory,
			target,
			builtIR.Targets,
			builtIR.GlobalVariables,
			effectiveParamValues,
			instanceInterpCtx,
			effectiveResolved,
		)
		if resolveErr != nil {
			return fmt.Errorf("node '%s': %w", nodeName, resolveErr)
		}

		absDir, absErr := filepath.Abs(workDir)
		if absErr != nil {
			return fmt.Errorf("node '%s': resolve directory: %w", nodeName, absErr)
		}

		runCommands, runArgvs := TargetExecutorExportRunSteps(steps)

		exportKey := targetExecutorExportGraphNodeKey(nodeName, instance.CacheKey)
		entry := TargetGraphExportEntry{
			CanonicalTarget: canonical,
			Directory:       absDir,
			RunCommands:     runCommands,
			RunArgvs:        runArgvs,
			DependsOn:       append([]string(nil), dependsOn...),
		}
		if instance.CacheKey != "" {
			entry.MatrixInstance = instance.CacheKey
		}
		if effectiveResolved.HasValues() || len(instance.Bindings) > 0 {
			entry.Parameters = TargetResolvedParametersExportForGraph(effectiveResolved)
			if entry.Parameters == nil {
				entry.Parameters = make(map[string]interface{}, len(instance.Bindings))
			}
			for key, value := range instance.Bindings {
				entry.Parameters[key] = value
			}
		}

		export.Targets[exportKey] = entry
	}

	return nil
}

func targetExecutorExportGraphNodeKey(nodeName string, matrixCacheKey string) string {
	if matrixCacheKey == "" {
		return nodeName
	}
	return nodeName + "#matrix:" + matrixCacheKey
}
