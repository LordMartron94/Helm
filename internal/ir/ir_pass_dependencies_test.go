package ir

import "testing"

func TestDependencyParameterSatisfiesDependencyListTargetParamRef(t *testing.T) {
	builder := &irBuilder{
		targets: map[string]HelmTarget{
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
		},
	}

	value := HelmParameterValue{
		Kind:            HelmParameterTargetParamRef,
		TargetParamName: "DEPS",
	}

	if !dependencyParameterSatisfiesDependencyList(builder, "wrapper", "DEPS", value) {
		t.Fatal("expected symmetric DEPS = DEPS forward to satisfy dependency list parameter")
	}
}
