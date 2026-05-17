package targetexecutor

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/google/shlex"
)

type TargetRunRequest struct {
	TargetName string
	StepIndex  int
	Command    string
	WorkDir    string
	Env        map[string]string
	// LiveStdout and LiveStderr, when non-nil, receive a copy of process output as it is produced.
	LiveStdout io.Writer
	LiveStderr io.Writer
}

type TargetRunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type TargetRunHandler func(req TargetRunRequest) (TargetRunResult, error)

func TargetExecutorDefaultRunHandler(req TargetRunRequest) (TargetRunResult, error) {
	var result TargetRunResult

	argv, err := shlex.Split(req.Command)
	if err != nil {
		return result, fmt.Errorf("target '%s' step %d: shlex split failed: %w", req.TargetName, req.StepIndex, err)
	}

	if len(argv) == 0 {
		return result, fmt.Errorf("target '%s' step %d: empty command after shlex split", req.TargetName, req.StepIndex)
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}

	cmd.Env = targetExecutorMergeEnv(req.Env)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = targetExecutorRunOutputWriter(&stdoutBuf, req.LiveStdout)
	cmd.Stderr = targetExecutorRunOutputWriter(&stderrBuf, req.LiveStderr)

	runErr := cmd.Run()
	result.Stdout = stdoutBuf.String()
	result.Stderr = stderrBuf.String()

	if runErr != nil {
		result.ExitCode = 1
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		return result, fmt.Errorf("target '%s' step %d: %w", req.TargetName, req.StepIndex, runErr)
	}

	return result, nil
}

func targetExecutorMergeEnv(targetEnv map[string]string) []string {
	if len(targetEnv) == 0 {
		return os.Environ()
	}

	merged := make(map[string]string)
	for _, entry := range os.Environ() {
		eq := strings.IndexByte(entry, '=')
		if eq < 0 {
			continue
		}
		merged[entry[:eq]] = entry[eq+1:]
	}

	for key, value := range targetEnv {
		merged[key] = value
	}

	out := make([]string, 0, len(merged))
	for key, value := range merged {
		out = append(out, key+"="+value)
	}

	return out
}

func targetExecutorRunOutputWriter(capture io.Writer, live io.Writer) io.Writer {
	if live == nil {
		return capture
	}
	if capture == nil {
		return live
	}
	return io.MultiWriter(capture, live)
}
