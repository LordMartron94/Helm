package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Invocation struct {
	HelmFilePath  string
	CommandFields []string
}

func IsBuiltinCommand(word string) bool {
	switch word {
	case "exit", "quit", "version", "help", "clean-cache", "set", "config", "run":
		return true
	default:
		return false
	}
}

func ParseInvocation(args []string) (Invocation, error) {
	if len(args) == 0 {
		return Invocation{}, nil
	}

	if IsBuiltinCommand(args[0]) {
		return Invocation{CommandFields: args}, nil
	}

	if isExplicitHelmFilePath(args[0]) {
		inv := Invocation{HelmFilePath: args[0]}
		if len(args) == 1 {
			return inv, nil
		}
		if !IsBuiltinCommand(args[1]) {
			return Invocation{}, fmt.Errorf(
				"unexpected argument %q after helm file (expected a command: exit, quit, version, help, clean-cache, set, config, run)",
				args[1],
			)
		}
		inv.CommandFields = args[1:]
		return inv, nil
	}

	return Invocation{}, fmt.Errorf(
		"unknown argument %q (expected a helm file path or command: exit, quit, version, help, clean-cache, set, config, run)",
		args[0],
	)
}

func isExplicitHelmFilePath(path string) bool {
	base := filepath.Base(path)
	if base == helmfileDefaultName {
		return true
	}
	if strings.EqualFold(filepath.Ext(path), ".helm") {
		return true
	}

	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
