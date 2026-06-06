package cli

import "testing"

func TestParseExportGraphTargetNames(t *testing.T) {
	names, err := parseExportGraphTargetNames("alpha,beta")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Fatalf("names: %#v", names)
	}

	names, err = parseExportGraphTargetNames(" alpha , beta ")
	if err != nil {
		t.Fatalf("parse spaced: %v", err)
	}
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Fatalf("spaced names: %#v", names)
	}

	_, err = parseExportGraphTargetNames("alpha,,beta")
	if err == nil {
		t.Fatal("expected error for empty segment")
	}
}

func TestExportGraphTargetCompletionPrefix(t *testing.T) {
	prefix, filter := exportGraphTargetCompletionPrefix("build_testbed,run_")
	if prefix != "build_testbed," || filter != "run_" {
		t.Fatalf("prefix=%q filter=%q", prefix, filter)
	}

	prefix, filter = exportGraphTargetCompletionPrefix("run_tests")
	if prefix != "" || filter != "run_tests" {
		t.Fatalf("single prefix=%q filter=%q", prefix, filter)
	}
}
