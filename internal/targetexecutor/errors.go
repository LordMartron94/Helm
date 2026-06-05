package targetexecutor

import "fmt"

const (
	ERROR_CONFIRM_CALLBACK_REQUIRED string = "EXEC_001"
	ERROR_TTY_CONFLICT              string = "EXEC_002"
)

// TargetExecutorProcessExitError is returned when a target subprocess exits non-zero.
type TargetExecutorProcessExitError struct {
	Target   string
	ExitCode int
	Err      error
}

func (e TargetExecutorProcessExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("target '%s' exited with code %d", e.Target, e.ExitCode)
}

func (e TargetExecutorProcessExitError) Unwrap() error {
	return e.Err
}

// TargetExecutorRunState tracks whether the entry target began executing runs.
type TargetExecutorRunState struct {
	EntryReached bool
}
