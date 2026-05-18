package cli

import (
	"bufio"
	"fmt"
	"helm/internal/ir"
	"helm/internal/targetexecutor"
	"helm/interpreter"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/shlex"
)

func CommandExecute(session *Session, line string, stdin io.Reader, stdout io.Writer) (bool, error) {
	fields, err := shlex.Split(line)
	if err != nil {
		return true, fmt.Errorf("could not parse command: %w", err)
	}
	return CommandExecuteFields(session, fields, stdin, stdout)
}

func CommandExecuteFields(session *Session, fields []string, stdin io.Reader, stdout io.Writer) (bool, error) {
	if stdin == nil {
		stdin = os.Stdin
	}
	if stdout == nil {
		stdout = os.Stdout
	}

	if len(fields) == 0 {
		return true, nil
	}

	verb := fields[0]
	args := fields[1:]

	switch verb {
	case "exit", "quit":
		return false, nil
	case "version":
		return true, PrintVersion(stdout)
	case "help":
		return true, commandHelp(session.UI, stdout, session.Catalog, args)
	case "clean-cache":
		return true, commandCleanCache(session.UI, stdout, session)
	case "set", "config":
		return true, commandSet(session, stdout, args)
	case "run":
		return true, commandRun(session, stdin, stdout, args)
	default:
		return true, fmt.Errorf("unknown command %q (try help)", verb)
	}
}

func commandHelp(ui *TerminalUI, stdout io.Writer, catalog TargetCatalog, args []string) error {
	if len(args) == 0 {
		if err := terminalUIWrite(stdout, ui, uiIntentHeading, "commands\n"); err != nil {
			return err
		}
		if err := printBuiltinCommand(stdout, ui, "exit, quit", "leave the shell"); err != nil {
			return err
		}
		if err := printBuiltinCommand(stdout, ui, "version", "print helm version"); err != nil {
			return err
		}
		if err := printBuiltinCommand(stdout, ui, "help [target]", "list commands or describe a target"); err != nil {
			return err
		}
		if err := printBuiltinCommand(stdout, ui, "clean-cache", "remove .helm/cache for this helm file"); err != nil {
			return err
		}
		if err := printBuiltinCommand(stdout, ui, "set [name on|off]", "show or change shell settings (e.g. set stream-runs off)"); err != nil {
			return err
		}
		if err := printBuiltinCommand(stdout, ui, "run [--bypass-cache] <target> [key=value ...]", "execute a target"); err != nil {
			return err
		}

		if _, err := fmt.Fprintln(stdout); err != nil {
			return err
		}
		if err := terminalUIWrite(stdout, ui, uiIntentHeading, "targets\n"); err != nil {
			return err
		}
		for _, name := range catalog.CanonicalNames() {
			if err := printTargetSummary(stdout, ui, catalog, name); err != nil {
				return err
			}
		}
		return nil
	}

	canonical, ok := catalog.ResolveTargetName(args[0])
	if !ok {
		return fmt.Errorf("unknown target %q", args[0])
	}
	return printTargetDetail(stdout, ui, catalog, canonical)
}

func printBuiltinCommand(stdout io.Writer, ui *TerminalUI, name, description string) error {
	if _, err := io.WriteString(stdout, "  "); err != nil {
		return err
	}
	if err := terminalUIWrite(stdout, ui, uiIntentAccent, name); err != nil {
		return err
	}
	if err := terminalUIWrite(stdout, ui, uiIntentMuted, "  "+description); err != nil {
		return err
	}
	_, err := io.WriteString(stdout, "\n")
	return err
}

func printTargetSummary(stdout io.Writer, ui *TerminalUI, catalog TargetCatalog, canonical string) error {
	entry, ok := catalog.Entry(canonical)
	if !ok {
		return fmt.Errorf("unknown target %q", canonical)
	}

	if _, err := io.WriteString(stdout, "  "); err != nil {
		return err
	}
	if err := terminalUIWrite(stdout, ui, uiIntentName, canonical); err != nil {
		return err
	}
	if err := terminalUIWrite(stdout, ui, uiIntentMuted, " — "+entry.HelpText); err != nil {
		return err
	}
	if len(entry.Aliases) > 0 {
		if err := terminalUIWrite(stdout, ui, uiIntentMuted, "  aliases: "+strings.Join(entry.Aliases, ", ")); err != nil {
			return err
		}
	}
	if paramPart := formatParameterList(entry.Parameters); paramPart != "" {
		if err := terminalUIWrite(stdout, ui, uiIntentMuted, paramPart); err != nil {
			return err
		}
	}
	_, err := io.WriteString(stdout, "\n")
	return err
}

func printTargetDetail(stdout io.Writer, ui *TerminalUI, catalog TargetCatalog, canonical string) error {
	entry, ok := catalog.Entry(canonical)
	if !ok {
		return fmt.Errorf("unknown target %q", canonical)
	}

	if err := terminalUIWrite(stdout, ui, uiIntentHeading, "target "); err != nil {
		return err
	}
	if err := terminalUIWrite(stdout, ui, uiIntentName, canonical+"\n"); err != nil {
		return err
	}
	if _, err := io.WriteString(stdout, "  "); err != nil {
		return err
	}
	if err := terminalUIWrite(stdout, ui, uiIntentMuted, entry.HelpText+"\n"); err != nil {
		return err
	}
	if len(entry.Aliases) > 0 {
		if _, err := io.WriteString(stdout, "  "); err != nil {
			return err
		}
		if err := terminalUIWrite(stdout, ui, uiIntentLabel, "aliases: "); err != nil {
			return err
		}
		if err := terminalUIWrite(stdout, ui, uiIntentMuted, strings.Join(entry.Aliases, ", ")+"\n"); err != nil {
			return err
		}
	}
	if len(entry.Parameters) == 0 {
		if _, err := io.WriteString(stdout, "  "); err != nil {
			return err
		}
		return terminalUIWrite(stdout, ui, uiIntentMuted, "parameters: (none)\n")
	}

	if _, err := io.WriteString(stdout, "  "); err != nil {
		return err
	}
	if err := terminalUIWrite(stdout, ui, uiIntentLabel, "parameters:\n"); err != nil {
		return err
	}
	for _, param := range entry.Parameters {
		if _, err := io.WriteString(stdout, "    "); err != nil {
			return err
		}
		if err := terminalUIWrite(stdout, ui, uiIntentName, param.Name); err != nil {
			return err
		}
		if param.Optional {
			if err := terminalUIWrite(stdout, ui, uiIntentMuted, " (optional)"); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(stdout, "\n"); err != nil {
			return err
		}
	}
	return nil
}

func formatParameterList(parameters []ir.HelmTargetParameter) string {
	if len(parameters) == 0 {
		return ""
	}
	names := make([]string, 0, len(parameters))
	for _, param := range parameters {
		if param.Optional {
			names = append(names, param.Name+"?")
		} else {
			names = append(names, param.Name)
		}
	}
	return "  params: " + strings.Join(names, ", ")
}

func commandCleanCache(ui *TerminalUI, stdout io.Writer, session *Session) error {
	cacheDir := filepath.Join(session.SourceDirectory(), ".helm", "cache")
	if err := os.RemoveAll(cacheDir); err != nil {
		return fmt.Errorf("clean-cache: %w", err)
	}
	if err := terminalUIWrite(stdout, ui, uiIntentAccent, "removed cache at "); err != nil {
		return err
	}
	return terminalUIWrite(stdout, ui, uiIntentPath, cacheDir+"\n")
}

func commandRun(session *Session, stdin io.Reader, stdout io.Writer, args []string) error {
	bypassCache := false
	rest := args
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		switch rest[0] {
		case "--bypass-cache":
			bypassCache = true
			rest = rest[1:]
		default:
			return fmt.Errorf("unknown run flag %q", rest[0])
		}
	}

	if len(rest) == 0 {
		return fmt.Errorf("usage: run [--bypass-cache] <target> [key=value ...]")
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

	parameters, err = promptMissingParameters(stdin, stdout, session.UI, entry, parameters)
	if err != nil {
		return err
	}

	invocations := map[string]targetexecutor.TargetInvocation{
		canonical: {Parameters: parameters},
	}

	opts := targetexecutor.TargetExecutorOptions{
		BypassCache:     bypassCache,
		StreamRunOutput: session.StreamRunOutput,
		RunHandler:      targetexecutor.TargetExecutorDefaultRunHandler,
		ConfirmDependency: func(dependent string, dep ir.HelmTargetDependency) (bool, error) {
			return promptConfirm(stdin, stdout, session.UI, dependent, dep)
		},
	}

	runErr := interpreter.HelmInterpreterExecuteTarget(
		session.Result,
		session.Renderer.Context(),
		canonical,
		invocations,
		opts,
	)
	session.FlushDiagnostics()

	if runErr != nil {
		return runErr
	}

	if err := terminalUIWrite(stdout, session.UI, uiIntentAccent, "finished "); err != nil {
		return err
	}
	return terminalUIWrite(stdout, session.UI, uiIntentName, canonical+"\n")
}

func parseRunParameters(entry TargetCatalogEntry, args []string) (map[string]string, error) {
	parameters := make(map[string]string)
	known := make(map[string]struct{}, len(entry.Parameters))
	for _, param := range entry.Parameters {
		known[param.Name] = struct{}{}
	}

	for _, arg := range args {
		key, value, found := strings.Cut(arg, "=")
		if !found {
			return nil, fmt.Errorf("expected key=value, got %q", arg)
		}
		if key == "" {
			return nil, fmt.Errorf("empty parameter name in %q", arg)
		}
		if _, ok := known[key]; !ok {
			return nil, fmt.Errorf("unknown parameter %q for target %s", key, entry.CanonicalName)
		}
		parameters[key] = value
	}

	return parameters, nil
}

func promptMissingParameters(
	stdin io.Reader,
	stdout io.Writer,
	ui *TerminalUI,
	entry TargetCatalogEntry,
	provided map[string]string,
) (map[string]string, error) {
	if provided == nil {
		provided = make(map[string]string)
	}

	reader := bufio.NewReader(stdin)
	for _, param := range entry.Parameters {
		if _, exists := provided[param.Name]; exists {
			continue
		}

		if err := writeParameterPrompt(stdout, ui, param.Name, param.Optional); err != nil {
			return nil, err
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		value := strings.TrimSpace(line)
		if value == "" {
			if param.Optional {
				continue
			}
			return nil, fmt.Errorf("parameter %s is required", param.Name)
		}
		provided[param.Name] = value
	}

	return provided, nil
}

func writeParameterPrompt(stdout io.Writer, ui *TerminalUI, paramName string, optional bool) error {
	if err := terminalUIWrite(stdout, ui, uiIntentLabel, "parameter "); err != nil {
		return err
	}
	if err := terminalUIWrite(stdout, ui, uiIntentName, paramName); err != nil {
		return err
	}
	if optional {
		if err := terminalUIWrite(stdout, ui, uiIntentMuted, " (optional, leave empty to skip)"); err != nil {
			return err
		}
	}
	_, err := io.WriteString(stdout, ": ")
	return err
}

func promptConfirm(
	stdin io.Reader,
	stdout io.Writer,
	ui *TerminalUI,
	dependent string,
	dep ir.HelmTargetDependency,
) (bool, error) {
	canonical := dep.TargetName
	if err := terminalUIWrite(stdout, ui, uiIntentName, dependent); err != nil {
		return false, err
	}
	if err := terminalUIWrite(stdout, ui, uiIntentMuted, " requires dependency "); err != nil {
		return false, err
	}
	if err := terminalUIWrite(stdout, ui, uiIntentAccent, canonical); err != nil {
		return false, err
	}
	if err := terminalUIWrite(stdout, ui, uiIntentLabel, ". proceed? [y/N]: "); err != nil {
		return false, err
	}

	reader := bufio.NewReader(stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes", nil
}
