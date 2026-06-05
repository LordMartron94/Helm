package interpreter

import (
	"strings"
	"testing"

	"helm/internal/ir"
	"lingua/helm"
	"signal"
)

func TestMultilineStringExtraction(t *testing.T) {
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

	casePath, err := resolveHelmCasePath("multiline_string.helm")
	if err != nil {
		t.Fatalf("resolve case: %v", err)
	}

	dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
		{Label: "ERROR", Weight: 20},
	})
	ctx := signal.SignalContextCreate(dispatcher)

	result := HelmInterpreterInterpretFile(helmInterp, casePath, ctx)
	if result.Error != nil {
		t.Fatalf("interpret: %v", result.Error)
	}
	if !result.BuiltIR.Succeeded {
		t.Fatal("expected IR build to succeed")
	}

	prefix, ok := result.BuiltIR.GlobalVariables["PREFIX"]
	if !ok {
		t.Fatal("missing PREFIX global")
	}
	if prefix.Kind != ir.HelmGlobalVarString {
		t.Fatalf("PREFIX kind: got %v", prefix.Kind)
	}
	wantPrefix := "\nline one\nline two\n"
	if prefix.StringValue != wantPrefix {
		t.Fatalf("PREFIX value:\nwant %q\ngot  %q", wantPrefix, prefix.StringValue)
	}

	demo, ok := result.BuiltIR.Targets["demo"]
	if !ok {
		t.Fatal("missing demo target")
	}
	wantHelp := "\n\tMulti-line\n\thelp text\n\t"
	if demo.HelpText != wantHelp {
		t.Fatalf("help text:\nwant %q\ngot  %q", wantHelp, demo.HelpText)
	}
	if len(demo.Steps) != 1 || demo.Steps[0].Kind != ir.TargetStepRun {
		t.Fatalf("steps: %#v", demo.Steps)
	}
	runText := demo.Steps[0].Run
	for _, fragment := range []string{"echo start", "line one", "line two", "echo end"} {
		if !strings.Contains(runText, fragment) {
			t.Fatalf("run missing %q:\n%s", fragment, runText)
		}
	}
}
