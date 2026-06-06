package targetexecutor

import (
	"helm/internal/ir"
	"testing"
)

func TestTargetExecutorBuildExecutionPlanParametricInstances(t *testing.T) {
	t.Parallel()

	builtIR := ir.HelmIR{
		SourceDirectory: "/tmp",
		Targets: map[string]ir.HelmTarget{
			"entry": {
				Name: "entry",
				DependsOn: []ir.HelmTargetDependency{
					{TargetName: "wrapper_a"},
					{TargetName: "wrapper_b"},
				},
			},
			"wrapper_a": {
				Name: "wrapper_a",
				DependsOn: []ir.HelmTargetDependency{
					{
						TargetName: "worker",
						Parameters: map[string]ir.HelmParameterValue{
							"OUT": {Kind: ir.HelmParameterScalar, Scalar: "a.out"},
						},
					},
				},
			},
			"wrapper_b": {
				Name: "wrapper_b",
				DependsOn: []ir.HelmTargetDependency{
					{
						TargetName: "worker",
						Parameters: map[string]ir.HelmParameterValue{
							"OUT": {Kind: ir.HelmParameterScalar, Scalar: "b.out"},
						},
					},
				},
			},
			"worker": {
				Name: "worker",
				Parameters: []ir.HelmTargetParameter{
					{Name: "OUT"},
				},
			},
		},
	}

	plan, err := TargetExecutorBuildExecutionPlan(builtIR, "entry", nil)
	if err != nil {
		t.Fatal(err)
	}

	workerNodes := 0
	for nodeID := range plan.NodeInvocations {
		if TargetExecutorExecutionNodeCanonical(nodeID) == "worker" {
			workerNodes++
		}
	}
	if workerNodes != 2 {
		t.Fatalf("expected 2 parametric worker instances, got %d (nodes %v)", workerNodes, plan.NodeInvocations)
	}
}

func TestTargetExecutorBuildExecutionPlanDedupesIdenticalParams(t *testing.T) {
	t.Parallel()

	builtIR := ir.HelmIR{
		SourceDirectory: "/tmp",
		Targets: map[string]ir.HelmTarget{
			"entry": {
				Name: "entry",
				DependsOn: []ir.HelmTargetDependency{
					{TargetName: "wrapper_a"},
					{TargetName: "wrapper_b"},
				},
			},
			"wrapper_a": {
				Name: "wrapper_a",
				DependsOn: []ir.HelmTargetDependency{
					{
						TargetName: "worker",
						Parameters: map[string]ir.HelmParameterValue{
							"OUT": {Kind: ir.HelmParameterScalar, Scalar: "same.out"},
						},
					},
				},
			},
			"wrapper_b": {
				Name: "wrapper_b",
				DependsOn: []ir.HelmTargetDependency{
					{
						TargetName: "worker",
						Parameters: map[string]ir.HelmParameterValue{
							"OUT": {Kind: ir.HelmParameterScalar, Scalar: "same.out"},
						},
					},
				},
			},
			"worker": {
				Name: "worker",
				Parameters: []ir.HelmTargetParameter{
					{Name: "OUT"},
				},
			},
		},
	}

	plan, err := TargetExecutorBuildExecutionPlan(builtIR, "entry", nil)
	if err != nil {
		t.Fatal(err)
	}

	workerNodes := 0
	for nodeID := range plan.NodeInvocations {
		if TargetExecutorExecutionNodeCanonical(nodeID) == "worker" {
			workerNodes++
		}
	}
	if workerNodes != 1 {
		t.Fatalf("expected 1 deduped worker instance, got %d", workerNodes)
	}
}
