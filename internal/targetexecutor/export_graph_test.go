package targetexecutor

import (
	"encoding/json"
	"strings"
	"testing"

	"helm/internal/ir"
)

func TestTargetExecutorExportExecutionGraph(t *testing.T) {
	builtIR := ir.HelmIR{
		SourceDirectory: "/tmp/helm-project",
		GlobalVariables: map[string]ir.HelmGlobalVariable{},
		Targets: map[string]ir.HelmTarget{
			"leaf": {
				Name: "leaf",
				Steps: []ir.HelmTargetStep{
					{Kind: ir.TargetStepRun, Run: "echo leaf"},
				},
			},
			"entry": {
				Name: "entry",
				DependsOn: []ir.HelmTargetDependency{
					{TargetName: "leaf"},
				},
				Steps: []ir.HelmTargetStep{
					{Kind: ir.TargetStepRun, Run: "echo entry"},
				},
			},
		},
		Succeeded: true,
	}

	export, err := TargetExecutorExportExecutionGraph(builtIR, "entry", nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	if export.EntryTarget != "entry" {
		t.Fatalf("entry_target: got %q", export.EntryTarget)
	}
	if len(export.Phases) == 0 {
		t.Fatal("expected non-empty phases")
	}

	leaf, ok := export.Targets["leaf"]
	if !ok {
		t.Fatalf("missing leaf in targets: %v", export.Targets)
	}
	if len(leaf.RunCommands) != 1 || leaf.RunCommands[0] != "echo leaf" {
		t.Fatalf("leaf run_commands: %#v", leaf.RunCommands)
	}
	if !strings.HasSuffix(leaf.Directory, "/tmp/helm-project") {
		t.Fatalf("leaf directory: %q", leaf.Directory)
	}

	entry, ok := export.Targets["entry"]
	if !ok {
		t.Fatalf("missing entry in targets")
	}
	if entry.RunCommands[0] != "echo entry" {
		t.Fatalf("entry run_commands: %#v", entry.RunCommands)
	}
	if len(entry.DependsOn) != 1 || entry.DependsOn[0] != "leaf" {
		t.Fatalf("entry depends_on: %#v", entry.DependsOn)
	}

	raw, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"run_commands"`) {
		t.Fatalf("json missing run_commands: %s", raw)
	}
}

func TestTargetExecutorExportExecutionGraphParametric(t *testing.T) {
	builtIR := ir.HelmIR{
		SourceDirectory: "/tmp/helm-project",
		GlobalVariables: map[string]ir.HelmGlobalVariable{},
		Targets: map[string]ir.HelmTarget{
			"base": {
				Name: "base",
				Parameters: []ir.HelmTargetParameter{
					{Name: "MSG"},
				},
				Steps: []ir.HelmTargetStep{
					{Kind: ir.TargetStepRun, Run: "echo ${MSG}"},
				},
			},
			"entry": {
				Name: "entry",
				DependsOn: []ir.HelmTargetDependency{
					{
						TargetName: "base",
						Parameters: map[string]ir.HelmParameterValue{
							"MSG": {Kind: ir.HelmParameterScalar, Scalar: "hello"},
						},
					},
				},
				Steps: []ir.HelmTargetStep{
					{Kind: ir.TargetStepRun, Run: "echo done"},
				},
			},
		},
		Succeeded: true,
	}

	export, err := TargetExecutorExportExecutionGraph(builtIR, "entry", nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	var baseKey string
	for key, entry := range export.Targets {
		if entry.CanonicalTarget == "base" && strings.Contains(key, "#") {
			baseKey = key
			if entry.RunCommands[0] != "echo hello" {
				t.Fatalf("base run_commands: %#v", entry.RunCommands)
			}
		}
	}
	if baseKey == "" {
		t.Fatalf("expected parametric base node key, targets: %v", export.Targets)
	}
}
