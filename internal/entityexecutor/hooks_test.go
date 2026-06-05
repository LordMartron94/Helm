package entityexecutor

import (
	"testing"

	"helm/internal/ir"
)

func TestEntityAdapterTargetHooksFromCompilePhase(t *testing.T) {
	builtIR := ir.HelmIR{
		Targets: map[string]ir.HelmTarget{
			"generate_compilation_database": {Name: "generate_compilation_database"},
		},
		Adapters: map[string]ir.HelmAdapterDecl{
			"c_executable": {
				Name: "c_executable",
				Phases: []ir.HelmAdapterPhase{
					{
						Name:            "_compile",
						TargetDependsOn: []string{"generate_compilation_database"},
					},
					{Name: "_link", DependsOn: []string{"_compile"}},
				},
			},
		},
		Entities: map[string]ir.HelmEntity{
			"//testbed:testbed": {
				Name:        "testbed",
				AdapterName: "c_executable",
			},
		},
	}

	hooks, err := EntityAdapterTargetHooks(builtIR, "//testbed:testbed")
	if err != nil {
		t.Fatal(err)
	}
	if len(hooks) != 1 || hooks[0] != "generate_compilation_database" {
		t.Fatalf("hooks = %#v", hooks)
	}
}
