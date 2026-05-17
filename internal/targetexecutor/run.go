package targetexecutor

import (
	"fmt"
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
}

type TargetRunHandler func(req TargetRunRequest) error

func TargetExecutorDefaultRunHandler(req TargetRunRequest) error {
	argv, err := shlex.Split(req.Command)
	if err != nil {
		return fmt.Errorf("target '%s' step %d: shlex split failed: %w", req.TargetName, req.StepIndex, err)
	}

	if len(argv) == 0 {
		return fmt.Errorf("target '%s' step %d: empty command after shlex split", req.TargetName, req.StepIndex)
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}

	cmd.Env = targetExecutorMergeEnv(req.Env)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("target '%s' step %d: %w", req.TargetName, req.StepIndex, err)
	}

	return nil
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
