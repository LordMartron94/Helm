package targetexecutor

import (
	"fmt"
	"helm/internal/ir"
	"strings"
	"testing"
)

func TestTargetExecutorDependencyBlockedParametricProducerFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	compileErr := fmt.Errorf("compile failed: test.h not found")

	builtIR := ir.HelmIR{
		SourceDirectory: dir,
		Targets: map[string]ir.HelmTarget{
			"_build_code": {
				Name: "_build_code",
				Parameters: []ir.HelmTargetParameter{
					{Name: "SOURCE_FILES"},
					{Name: "OUT_NAME"},
					{Name: "DEFINES"},
					{Name: "COMPILER_FLAGS"},
					{Name: "INCLUDE_FLAGS"},
					{Name: "LINKER_FLAGS"},
				},
				Artifacts: &ir.HelmArtifacts{
					Outputs: []ir.HelmArtifactInput{
						{Kind: ir.ArtifactInputString, Literal: "build/${OUT_NAME}"},
					},
				},
				Steps: []ir.HelmTargetStep{
					{Kind: ir.TargetStepRun, Run: "false"},
				},
			},
			"build_application": {
				Name: "build_application",
				Parameters: []ir.HelmTargetParameter{
					{Name: "SOURCE_FILES"},
					{Name: "OUT_NAME"},
					{Name: "DEFINES"},
					{Name: "INCLUDE_FLAGS"},
					{Name: "LINKER_FLAGS"},
				},
				Artifacts: &ir.HelmArtifacts{
					Outputs: []ir.HelmArtifactInput{
						{Kind: ir.ArtifactInputString, Literal: "build/${OUT_NAME}"},
					},
				},
				DependsOn: []ir.HelmTargetDependency{
					{
						TargetName: "_build_code",
						Parameters: map[string]ir.HelmParameterValue{
							"SOURCE_FILES":     {Kind: ir.HelmParameterScalar, Scalar: "main.c"},
							"OUT_NAME":           {Kind: ir.HelmParameterScalar, Scalar: "testbed"},
							"DEFINES":            {Kind: ir.HelmParameterScalar},
							"COMPILER_FLAGS":     {Kind: ir.HelmParameterScalar, Scalar: "-g"},
							"INCLUDE_FLAGS":      {Kind: ir.HelmParameterScalar},
							"LINKER_FLAGS":       {Kind: ir.HelmParameterScalar},
						},
					},
				},
			},
			"build_testbed": {
				Name: "build_testbed",
				Artifacts: &ir.HelmArtifacts{
					Outputs: []ir.HelmArtifactInput{
						{Kind: ir.ArtifactInputString, Literal: "build/testbed"},
					},
				},
				DependsOn: []ir.HelmTargetDependency{
					{
						TargetName: "build_application",
						Parameters: map[string]ir.HelmParameterValue{
							"SOURCE_FILES": {Kind: ir.HelmParameterScalar, Scalar: "main.c"},
							"OUT_NAME":       {Kind: ir.HelmParameterScalar, Scalar: "testbed"},
							"DEFINES":        {Kind: ir.HelmParameterScalar},
							"INCLUDE_FLAGS":  {Kind: ir.HelmParameterScalar},
							"LINKER_FLAGS":   {Kind: ir.HelmParameterScalar},
						},
					},
				},
			},
		},
	}

	opts := TargetExecutorOptions{
		DisableArtifactCache: true,
		RunHandler: func(req TargetRunRequest) (TargetRunResult, error) {
			if req.TargetName == "_build_code" {
				return TargetRunResult{}, compileErr
			}
			return TargetRunResult{}, nil
		},
	}

	err := TargetExecutorRunGraph(builtIR, "build_testbed", nil, opts)
	if err == nil {
		t.Fatal("expected blocked error")
	}

	message := err.Error()
	if !strings.Contains(message, "blocked") {
		t.Fatalf("expected blocked error, got %v", err)
	}
	if !strings.Contains(message, "_build_code") {
		t.Fatalf("expected _build_code in chain, got %v", err)
	}
	if !strings.Contains(message, "compile failed") {
		t.Fatalf("expected root compile error, got %v", err)
	}
	if strings.Contains(message, "fingerprint file") {
		t.Fatalf("expected dependency block, not fingerprint error, got %v", err)
	}
}

func TestTargetExecutorDependencyNodesMatchesPlanOutgoing(t *testing.T) {
	t.Parallel()

	builtIR := ir.HelmIR{
		SourceDirectory: "/tmp",
		Targets: map[string]ir.HelmTarget{
			"build_application": {
				Name: "build_application",
				DependsOn: []ir.HelmTargetDependency{
					{
						TargetName: "_build_code",
						Parameters: map[string]ir.HelmParameterValue{
							"OUT": {Kind: ir.HelmParameterScalar, Scalar: "testbed"},
						},
					},
				},
			},
			"_build_code": {
				Name: "_build_code",
				Parameters: []ir.HelmTargetParameter{{Name: "OUT"}},
			},
		},
	}

	plan, err := TargetExecutorBuildExecutionPlan(builtIR, "build_application", nil)
	if err != nil {
		t.Fatal(err)
	}

	nodes, err := targetExecutorDependencyNodes(plan, builtIR, "build_application", TargetInvocation{})
	if err != nil {
		t.Fatal(err)
	}

	if len(nodes) != 1 {
		t.Fatalf("expected 1 dependency node, got %v", nodes)
	}
	if nodes[0] == "_build_code" {
		t.Fatalf("expected parametric instance id, got canonical %q", nodes[0])
	}
	if TargetExecutorExecutionNodeCanonical(nodes[0]) != "_build_code" {
		t.Fatalf("expected _build_code instance, got %q", nodes[0])
	}
}
