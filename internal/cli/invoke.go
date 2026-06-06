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

var builtinCommands = []string{
	"clean-cache",
	"config",
	"exit",
	"export-graph",
	"help",
	"quit",
	"run",
	"set",
	"version",
}

func BuiltinCommands() []string {
	out := make([]string, len(builtinCommands))
	copy(out, builtinCommands)
	return out
}

func IsBuiltinCommand(word string) bool {
	for _, cmd := range builtinCommands {
		if cmd == word {
			return true
		}
	}
	return false
}

func builtinCommandsList() string {
	return strings.Join(builtinCommands, ", ")
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
				"unexpected argument %q after helm file (expected a command: %s)",
				args[1],
				builtinCommandsList(),
			)
		}
		inv.CommandFields = args[1:]
		return inv, nil
	}

	return Invocation{}, fmt.Errorf(
		"unknown argument %q (expected a helm file path or command: %s)",
		args[0],
		builtinCommandsList(),
	)
}

func isExplicitHelmFilePath(path string) bool {
	base := filepath.Base(path)
	if base == helmfileDefaultName {
		return true
	}

	info, err := os.Stat(path)
	if err == nil {
		return !info.IsDir()
	}

	return strings.EqualFold(filepath.Ext(path), ".helm")
}

func isHelmFileName(name string) bool {
	if strings.HasPrefix(name, ".") {
		return false
	}
	if name == helmfileDefaultName {
		return true
	}
	return strings.EqualFold(filepath.Ext(name), ".helm")
}
