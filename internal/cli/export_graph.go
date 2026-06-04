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
				return fmt.Errorf("usage: export-graph [-o path] <target> [key=value ...]")
			}
			outputPath = rest[1]
			rest = rest[2:]
		default:
			return fmt.Errorf("unknown export-graph flag %q", rest[0])
		}
	}

	if len(rest) == 0 {
		return fmt.Errorf("usage: export-graph [-o path] <target> [key=value ...]")
	}

	targetName := rest[0]
	rest = rest[1:]

	canonical, ok := session.Catalog.ResolveTargetName(targetName)
	if !ok {
		return fmt.Errorf("unknown target %q", targetName)
	}

	entry, ok := session.Catalog.Entry(canonical)
	if !ok {
		return fmt.Errorf("unknown target %q", targetName)
	}

	parameters, err := parseRunParameters(entry, rest)
	if err != nil {
		return err
	}

	invocations := map[string]targetexecutor.TargetInvocation{
		canonical: targetexecutor.TargetInvocationWithScalars(parameters),
	}

	payload, err := interpreter.HelmInterpreterExportExecutionGraphJSON(
		session.Result,
		canonical,
		invocations,
	)
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
