package targetexecutor

import (
	"testing"

	"helm/internal/ir"
)

func TestTargetExecutorDependencyBlockedSkipsEntityLabels(t *testing.T) {
	t.Parallel()

	builtIR := ir.HelmIR{
		SourceDirectory: "/ws",
		Targets: map[string]ir.HelmTarget{
			"run_tests": {
				Name: "run_tests",
				DependsOn: []ir.HelmTargetDependency{
					{
						TargetName:  "//testbed:testbed",
						EntityLabel: &ir.HelmLabel{Path: "testbed", Name: "testbed"},
					},
				},
			},
		},
	}

	plan, err := TargetExecutorBuildExecutionPlan(builtIR, "run_tests", nil)
	if err != nil {
		t.Fatal(err)
	}

	results := map[string]error{"run_tests": nil}
	err = targetExecutorDependencyBlocked(plan, builtIR, "run_tests", TargetInvocation{}, results)
	if err != nil {
		t.Fatalf("entity label dependency should be skipped, got %v", err)
	}
}
