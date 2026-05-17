package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
)

func RunShell(session *Session) error {
	reader := bufio.NewReader(os.Stdin)

	for {
		if err := terminalUIWritePrompt(os.Stdout, session.UI); err != nil {
			return err
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(os.Stdout)
				return nil
			}
			return err
		}

		line = trimLine(line)
		if line == "" {
			continue
		}

		continuing, err := CommandExecute(session, line, os.Stdin, os.Stdout)
		if err != nil {
			if writeErr := writeCommandError(os.Stderr, session.UI, err); writeErr != nil {
				return writeErr
			}
		}
		if !continuing {
			return nil
		}
	}
}

func writeCommandError(w io.Writer, ui *TerminalUI, err error) error {
	if err := terminalUIWrite(w, ui, uiIntentError, "error"); err != nil {
		return err
	}
	if _, writeErr := fmt.Fprintf(w, ": %v\n", err); writeErr != nil {
		return writeErr
	}
	return nil
}

func trimLine(line string) string {
	for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
		line = line[:len(line)-1]
	}
	for len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
		line = line[1:]
	}
	for len(line) > 0 && (line[len(line)-1] == ' ' || line[len(line)-1] == '\t') {
		line = line[:len(line)-1]
	}
	return line
}
