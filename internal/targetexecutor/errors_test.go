package targetexecutor

import (
	"errors"
	"testing"
)

func TestTargetExecutorProcessExitError(t *testing.T) {
	root := errors.New("exit status 2")
	exitErr := TargetExecutorProcessExitError{
		Target:   "demo",
		ExitCode: 2,
		Err:      root,
	}

	if exitErr.ExitCode != 2 {
		t.Fatalf("exit code: got %d", exitErr.ExitCode)
	}
	if !errors.Is(exitErr, root) {
		t.Fatal("expected unwrap to root error")
	}
}
