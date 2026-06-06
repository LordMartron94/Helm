package targetexecutor

import (
	"helm/internal/ir"
	"testing"
)

func TestTargetExecutorEffectiveDependsOnExpandsParameterList(t *testing.T) {
	t.Parallel()

	builtIR := ir.HelmIR{
		SourceDirectory: "/tmp",
		Targets: map[string]ir.HelmTarget{
			"worker": {
				Name: "worker",
				Parameters: []ir.HelmTargetParameter{
					{Name: "DEPS", DependencyList: true},
				},
				DependsOnParamNames: []string{"DEPS"},
			},
			"helper": {Name: "helper"},
			"leaf":   {Name: "leaf"},
		},
	}

	inv := TargetInvocation{
		Parameters: map[string]ir.HelmParameterValue{
			"DEPS": {
				Kind: ir.HelmParameterDependencyList,
				Dependencies: []ir.HelmTargetDependency{
					{TargetName: "helper"},
					{TargetName: "leaf"},
				},
			},
		},
	}

	deps, err := targetExecutorEffectiveDependsOn(builtIR, builtIR.Targets["worker"], inv, TargetResolvedParametersEmpty())
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 2 {
		t.Fatalf("expected 2 dependencies, got %d", len(deps))
	}
	if deps[0].TargetName != "helper" || deps[1].TargetName != "leaf" {
		t.Fatalf("unexpected deps: %+v", deps)
	}
}

func TestTargetExecutorBuildExecutionPlanWithDependencyParameter(t *testing.T) {
	t.Parallel()

	builtIR := ir.HelmIR{
		SourceDirectory: "/tmp",
		Targets: map[string]ir.HelmTarget{
			"entry": {
				Name: "entry",
				DependsOn: []ir.HelmTargetDependency{
					{
						TargetName: "worker",
						Parameters: map[string]ir.HelmParameterValue{
							"DEPS": {
								Kind: ir.HelmParameterDependencyList,
								Dependencies: []ir.HelmTargetDependency{
									{TargetName: "helper"},
								},
							},
						},
					},
				},
			},
			"worker": {
				Name: "worker",
				Parameters: []ir.HelmTargetParameter{
					{Name: "DEPS", DependencyList: true},
				},
				DependsOnParamNames: []string{"DEPS"},
			},
			"helper": {Name: "helper"},
		},
	}

	plan, err := TargetExecutorBuildExecutionPlan(builtIR, "entry", nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.NodeInvocations) < 1 {
		t.Fatalf("expected parametric worker instance, got %v", plan.NodeInvocations)
	}

	workerNodes := 0
	for nodeID := range plan.NodeInvocations {
		if TargetExecutorExecutionNodeCanonical(nodeID) == "worker" {
			workerNodes++
		}
	}
	if workerNodes != 1 {
		t.Fatalf("expected 1 worker instance, got %d", workerNodes)
	}
}
