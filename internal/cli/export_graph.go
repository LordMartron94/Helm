package cli

import (
	"fmt"
	"helm/internal/targetexecutor"
	"helm/interpreter"
	"io"
	"os"
	"strings"
)

func commandExportGraph(session *Session, stdout io.Writer, args []string) error {
	outputPath := ""
	rest := args
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		switch rest[0] {
		case "--output", "-o":
			if len(rest) < 2 {
				return fmt.Errorf("usage: export-graph [-o path] <target>[,<target>...] [key=value ...]")
			}
			outputPath = rest[1]
			rest = rest[2:]
		default:
			return fmt.Errorf("unknown export-graph flag %q", rest[0])
		}
	}

	if len(rest) == 0 {
		return fmt.Errorf("usage: export-graph [-o path] <target>[,<target>...] [key=value ...]")
	}

	targetNames, err := parseExportGraphTargetNames(rest[0])
	if err != nil {
		return err
	}
	rest = rest[1:]

	if len(targetNames) > 1 && len(rest) > 0 {
		return fmt.Errorf("key=value parameters are only supported when exporting a single target")
	}

	canonicalNames := make([]string, 0, len(targetNames))
	invocations := make(map[string]targetexecutor.TargetInvocation, len(targetNames))

	for _, targetName := range targetNames {
		canonical, ok := session.Catalog.ResolveTargetName(targetName)
		if !ok {
			return fmt.Errorf("unknown target %q", targetName)
		}

		entry, ok := session.Catalog.Entry(canonical)
		if !ok {
			return fmt.Errorf("unknown target %q", targetName)
		}

		canonicalNames = append(canonicalNames, canonical)

		if len(targetNames) == 1 {
			parameters, paramErr := parseRunParameters(entry, rest)
			if paramErr != nil {
				return paramErr
			}
			invocations[canonical] = targetexecutor.TargetInvocationWithScalars(parameters)
		}
	}

	var payload []byte
	if len(canonicalNames) == 1 {
		payload, err = interpreter.HelmInterpreterExportExecutionGraphJSON(
			session.Result,
			canonicalNames[0],
			invocations,
		)
	} else {
		payload, err = interpreter.HelmInterpreterExportExecutionGraphBundleJSON(
			session.Result,
			canonicalNames,
			invocations,
		)
	}
	if err != nil {
		session.FlushDiagnostics()
		return err
	}
	session.FlushDiagnostics()

	if outputPath == "" {
		_, err = stdout.Write(payload)
		if err != nil {
			return err
		}
		if len(payload) > 0 && payload[len(payload)-1] != '\n' {
			_, err = io.WriteString(stdout, "\n")
		}
		return err
	}

	return os.WriteFile(outputPath, payload, 0o644)
}

func parseExportGraphTargetNames(arg string) ([]string, error) {
	segments := strings.Split(arg, ",")
	names := make([]string, 0, len(segments))

	for _, segment := range segments {
		name := strings.TrimSpace(segment)
		if name == "" {
			return nil, fmt.Errorf("invalid target list %q: empty entry between commas", arg)
		}
		names = append(names, name)
	}

	if len(names) == 0 {
		return nil, fmt.Errorf("at least one target is required")
	}

	return names, nil
}

func exportGraphTargetCompletionPrefix(cur string) (completedPrefix string, filter string) {
	index := strings.LastIndex(cur, ",")
	if index < 0 {
		return "", cur
	}
	return cur[:index+1], strings.TrimLeft(cur[index+1:], " \t")
}
