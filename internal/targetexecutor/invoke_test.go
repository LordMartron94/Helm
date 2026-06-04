package targetexecutor

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestTargetExecutorDependencyBlockedErrorFormat(t *testing.T) {
	t.Parallel()

	root := fmt.Errorf("target %q step %d: exit status 1", "_build_code", 2)

	err := TargetExecutorFormatDependencyBlockedError(
		"build_application",
		"_build_code",
		root,
	)
	err = TargetExecutorFormatDependencyBlockedError("build_testbed", "build_application", err)
	err = TargetExecutorFormatDependencyBlockedError("run_tests", "build_testbed", err)

	message := err.Error()
	want := `target "run_tests" blocked (build_testbed → build_application → _build_code): target "_build_code" step 2: exit status 1`
	if message != want {
		t.Fatalf("unexpected message:\n  got:  %s\n  want: %s", message, want)
	}
	if strings.Contains(message, "dependency '") {
		t.Fatalf("expected compact chain, got nested wording: %s", message)
	}

	var blocked *TargetExecutorDependencyBlockedError
	if !errors.As(err, &blocked) {
		t.Fatal("expected structured blocked error")
	}
	if !errors.Is(err, root) {
		t.Fatal("expected unwrap to root cause")
	}
}
