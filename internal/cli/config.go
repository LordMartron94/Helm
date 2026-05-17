package cli

import (
	"fmt"
	"io"
	"strings"
)

func commandSet(session *Session, stdout io.Writer, args []string) error {
	if len(args) == 0 {
		return printSessionConfig(session, stdout)
	}

	switch strings.ToLower(args[0]) {
	case "stream-runs":
		if len(args) < 2 {
			return fmt.Errorf("usage: set stream-runs on|off")
		}
		value, err := parseBoolSetting(args[1])
		if err != nil {
			return fmt.Errorf("stream-runs: %w", err)
		}
		session.StreamRunOutput = value
		return printSetting(stdout, session.UI, "stream-runs", formatBoolSetting(value))
	default:
		return fmt.Errorf("unknown setting %q (try: set stream-runs on|off)", args[0])
	}
}

func printSessionConfig(session *Session, stdout io.Writer) error {
	if err := terminalUIWrite(stdout, session.UI, uiIntentHeading, "settings\n"); err != nil {
		return err
	}
	return printSetting(stdout, session.UI, "stream-runs", formatBoolSetting(session.StreamRunOutput))
}

func printSetting(stdout io.Writer, ui *TerminalUI, name, value string) error {
	if _, err := io.WriteString(stdout, "  "); err != nil {
		return err
	}
	if err := terminalUIWrite(stdout, ui, uiIntentName, name); err != nil {
		return err
	}
	if err := terminalUIWrite(stdout, ui, uiIntentMuted, " = "); err != nil {
		return err
	}
	if err := terminalUIWrite(stdout, ui, uiIntentAccent, value); err != nil {
		return err
	}
	_, err := io.WriteString(stdout, "\n")
	return err
}

func formatBoolSetting(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

func parseBoolSetting(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "on", "true", "yes", "1":
		return true, nil
	case "off", "false", "no", "0":
		return false, nil
	default:
		return false, fmt.Errorf("expected on or off, got %q", raw)
	}
}
