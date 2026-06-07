package interpreter

import (
	"os"
	"path/filepath"
	"testing"

	"helm/internal/ir"
	"lingua/helm"
	"signal"
)

func TestRunArgvExtraction(t *testing.T) {
	t.Parallel()

	specPath, releaseSpec, err := helm.ResolveHelmLSpecPath()
	if err != nil {
		t.Fatalf("resolve spec: %v", err)
	}
	defer releaseSpec()

	helmInterp, err := HelmInterpreterTryCreate(specPath)
	if err != nil {
		t.Fatalf("create interpreter: %v", err)
	}
	defer HelmInterpreterDestroy(helmInterp)

	dir := t.TempDir()
	helmPath := filepath.Join(dir, "run_argv.helm")
	content := `FILES = [
    "one.c",
    "two.c"
]

target link(SOURCE_FILES) {
    help = "links objects"
    artifacts { volatile = true }
    run [
        "tools/link.sh",
        "build/out",
        param SOURCE_FILES
    ]
}
`
	if err := os.WriteFile(helmPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write helm: %v", err)
	}

	dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
		{Label: "ERROR", Weight: 20},
	})
	ctx := signal.SignalContextCreate(dispatcher)

	result := HelmInterpreterInterpretFile(helmInterp, helmPath, ctx)
	if result.Error != nil {
		t.Fatalf("interpret: %v", result.Error)
	}
	if !result.BuiltIR.Succeeded {
		t.Fatal("expected IR build to succeed")
	}

	target, ok := result.BuiltIR.Targets["link"]
	if !ok {
		t.Fatal("missing link target")
	}
	if len(target.Steps) != 1 {
		t.Fatalf("steps: %#v", target.Steps)
	}

	run := target.Steps[0].Run
	if !ir.HelmRunCommandIsArgv(run) {
		t.Fatalf("expected argv run, got %#v", run)
	}
	if len(run.Argv) != 3 {
		t.Fatalf("argv template: %#v", run.Argv)
	}
	if run.Argv[0].Literal != "tools/link.sh" || run.Argv[1].Literal != "build/out" {
		t.Fatalf("literals: %#v", run.Argv)
	}
	if run.Argv[2].ParamName != "SOURCE_FILES" {
		t.Fatalf("param splice: %#v", run.Argv[2])
	}
}

func TestWhenRunArgvExtraction(t *testing.T) {
	t.Parallel()

	specPath, releaseSpec, err := helm.ResolveHelmLSpecPath()
	if err != nil {
		t.Fatalf("resolve spec: %v", err)
	}
	defer releaseSpec()

	helmInterp, err := HelmInterpreterTryCreate(specPath)
	if err != nil {
		t.Fatalf("create interpreter: %v", err)
	}
	defer HelmInterpreterDestroy(helmInterp)

	dir := t.TempDir()
	helmPath := filepath.Join(dir, "when_argv.helm")
	content := `FILES = [
    "one.c",
    "two.c"
]

target link_when(SOURCE_FILES) {
    help = "conditional argv link"
    artifacts { volatile = true }

    when defined(SOURCE_FILES) {
        run [
            "tools/link.sh",
            param SOURCE_FILES
        ]
    }
}
`
	if err := os.WriteFile(helmPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write helm: %v", err)
	}

	dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
		{Label: "ERROR", Weight: 20},
	})
	ctx := signal.SignalContextCreate(dispatcher)

	result := HelmInterpreterInterpretFile(helmInterp, helmPath, ctx)
	if result.Error != nil {
		t.Fatalf("interpret: %v", result.Error)
	}
	if !result.BuiltIR.Succeeded {
		t.Fatal("expected IR build to succeed")
	}

	target, ok := result.BuiltIR.Targets["link_when"]
	if !ok {
		t.Fatal("missing link_when target")
	}
	if len(target.Steps) != 1 || target.Steps[0].Kind != ir.TargetStepWhen {
		t.Fatalf("steps: %#v", target.Steps)
	}

	when := target.Steps[0].When
	if when == nil || len(when.Runs) != 1 {
		t.Fatalf("when runs: %#v", when)
	}

	run := when.Runs[0]
	if !ir.HelmRunCommandIsArgv(run) {
		t.Fatalf("expected argv run, got %#v", run)
	}
	if len(run.Argv) != 2 || run.Argv[0].Literal != "tools/link.sh" {
		t.Fatalf("argv template: %#v", run.Argv)
	}
	if run.Argv[1].ParamName != "SOURCE_FILES" {
		t.Fatalf("param splice: %#v", run.Argv[1])
	}
}

func TestWhenGlobalConditionExtraction(t *testing.T) {
	t.Parallel()

	specPath, releaseSpec, err := helm.ResolveHelmLSpecPath()
	if err != nil {
		t.Fatalf("resolve spec: %v", err)
	}
	defer releaseSpec()

	helmInterp, err := HelmInterpreterTryCreate(specPath)
	if err != nil {
		t.Fatalf("create interpreter: %v", err)
	}
	defer HelmInterpreterDestroy(helmInterp)

	dir := t.TempDir()
	helmPath := filepath.Join(dir, "when_global.helm")
	content := `VERSION = "2.1.5"

target publish() {
    help = "publish when version global is set"
    artifacts { volatile = true }

    when defined(VERSION) {
        run "echo ${VERSION}"
    }
}
`
	if err := os.WriteFile(helmPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write helm: %v", err)
	}

	dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
		{Label: "ERROR", Weight: 20},
	})
	ctx := signal.SignalContextCreate(dispatcher)

	result := HelmInterpreterInterpretFile(helmInterp, helmPath, ctx)
	if result.Error != nil {
		t.Fatalf("interpret: %v", result.Error)
	}
	if !result.BuiltIR.Succeeded {
		t.Fatal("expected IR build to succeed")
	}

	target, ok := result.BuiltIR.Targets["publish"]
	if !ok {
		t.Fatal("missing publish target")
	}
	if len(target.Steps) != 1 || target.Steps[0].Kind != ir.TargetStepWhen {
		t.Fatalf("steps: %#v", target.Steps)
	}
	if target.Steps[0].When == nil || target.Steps[0].When.Parameter != "VERSION" {
		t.Fatalf("when condition: %#v", target.Steps[0].When)
	}
}
