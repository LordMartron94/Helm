package ir

import "testing"

func TestDependencyParameterSatisfiesDependencyListTargetParamRef(t *testing.T) {
	t.Parallel()

	targets := map[string]HelmTarget{
		"wrapper": {
			Name: "wrapper",
			Parameters: []HelmTargetParameter{
				{Name: "DEPS"},
			},
		},
		"inner": {
			Name: "inner",
			Parameters: []HelmTargetParameter{
				{Name: "DEPS", DependencyList: true},
			},
		},
	}

	value := HelmParameterValue{
		Kind:            HelmParameterTargetParamRef,
		TargetParamName: "DEPS",
	}

	if !dependencyParameterSatisfiesDependencyList(targets, "wrapper", "DEPS", value) {
		t.Fatal("expected symmetric DEPS = DEPS forward to satisfy dependency list parameter")
	}
}

/*
TestIrFixpointPropagateDependencyListMarksChain verifies that DependencyList marking propagates
through a multi-hop DEPS = DEPS forward chain when only the leaf splices depends_on [ param DEPS ],
even if outer edges are visited before inner edges (the order that broke optional DEPENDENCIES?).
*/
func TestIrFixpointPropagateDependencyListMarksChain(t *testing.T) {
	t.Parallel()

	targets := map[string]HelmTarget{
		"outer": {
			Name: "outer",
			Parameters: []HelmTargetParameter{
				{Name: "DEPS", Optional: true},
			},
			DependsOn: []HelmTargetDependency{
				{
					TargetName: "middle",
					Parameters: map[string]HelmParameterValue{
						"DEPS": {
							Kind:            HelmParameterTargetParamRef,
							TargetParamName: "DEPS",
						},
					},
				},
			},
		},
		"middle": {
			Name: "middle",
			Parameters: []HelmTargetParameter{
				{Name: "DEPS", Optional: true},
			},
			DependsOn: []HelmTargetDependency{
				{
					TargetName: "leaf",
					Parameters: map[string]HelmParameterValue{
						"DEPS": {
							Kind:            HelmParameterTargetParamRef,
							TargetParamName: "DEPS",
						},
					},
				},
			},
		},
		"leaf": {
			Name: "leaf",
			Parameters: []HelmTargetParameter{
				{Name: "DEPS", Optional: true, DependencyList: true},
			},
			DependsOnParamNames: []string{"DEPS"},
		},
	}

	builder := &irBuilder{targets: targets}
	irFixpointPropagateDependencyListMarks(builder)

	for _, name := range []string{"middle", "outer"} {
		param, ok := irTargetParameterByName(builder.targets[name].Parameters, "DEPS")
		if !ok {
			t.Fatalf("target %q missing DEPS parameter", name)
		}
		if !param.DependencyList {
			t.Fatalf("target %q: DEPS must be marked DependencyList after fixpoint propagation", name)
		}
	}
}
