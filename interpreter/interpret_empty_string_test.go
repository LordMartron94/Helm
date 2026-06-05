package interpreter

import (
	"os"
	"path/filepath"
	"testing"

	"helm/internal/ir"
	"lingua/helm"
	"signal"
)

func TestEmptyStringParsing(t *testing.T) {
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

	casePath, err := resolveHelmCasePath("empty_string.helm")
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

	empty, ok := result.BuiltIR.GlobalVariables["EMPTY"]
	if !ok {
		t.Fatal("missing EMPTY global")
	}
	if empty.Kind != ir.HelmGlobalVarString || empty.StringValue != "" {
		t.Fatalf("EMPTY: kind=%v value=%q", empty.Kind, empty.StringValue)
	}

	demo, ok := result.BuiltIR.Targets["demo"]
	if !ok {
		t.Fatal("missing demo target")
	}
	if demo.HelpText != "" {
		t.Fatalf("help: got %q", demo.HelpText)
	}
	if len(demo.Steps) != 1 || demo.Steps[0].Run.String != "" {
		t.Fatalf("run step: %#v", demo.Steps)
	}
}

func resolveHelmCasePath(name string) (string, error) {
	if p := os.Getenv("HELM_TEST_CASES_DIR"); p != "" {
		return filepath.Join(p, name), nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dir := wd
	for {
		candidate := filepath.Join(dir, "tools/helm/tests/cases", name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", os.ErrNotExist
}
