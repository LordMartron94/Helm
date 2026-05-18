package cli

import (
	"io"
	"os"
)

type RunConfig struct {
	HelmFilePath    string
	LSpecPath       string
	ColorMode       ColorMode
	StreamRunOutput *bool
	CommandFields   []string
}

func Run(config RunConfig) error {
	helmFile, err := DiscoverHelmFile(config.HelmFilePath)
	if err != nil {
		return err
	}

	lSpecPath := config.LSpecPath
	if lSpecPath == "" {
		lSpecPath, err = ResolveHelmLSpecPath()
		if err != nil {
			return err
		}
	}

	colorMode := config.ColorMode
	if colorMode == "" {
		colorMode = ResolveDefaultColorMode()
	}

	session, err := SessionCreate(SessionConfig{
		HelmFile:        helmFile,
		LSpecPath:       lSpecPath,
		ColorMode:       colorMode,
		StreamRunOutput: config.StreamRunOutput,
	})
	if err != nil {
		return err
	}
	defer SessionDestroy(session)

	if err := printStartupBanner(os.Stderr, session.UI, session, helmFile, session.CacheDirectory()); err != nil {
		return err
	}

	if len(config.CommandFields) > 0 {
		_, err := CommandExecuteFields(session, config.CommandFields, os.Stdin, os.Stdout)
		return err
	}

	return RunShell(session)
}

func printStartupBanner(w io.Writer, ui *TerminalUI, session *Session, helmFile, cacheDir string) error {
	if err := terminalUIWrite(w, ui, uiIntentHeading, "session\n"); err != nil {
		return err
	}
	if err := writeStartupLine(w, ui, "  helm file  ", helmFile); err != nil {
		return err
	}
	if err := writeStartupLine(w, ui, "  cache      ", cacheDir); err != nil {
		return err
	}
	return writeStartupLine(w, ui, "  stream-runs", formatBoolSetting(session.StreamRunOutput))
}

func writeStartupLine(w io.Writer, ui *TerminalUI, label, value string) error {
	if err := terminalUIWrite(w, ui, uiIntentMuted, label); err != nil {
		return err
	}
	if err := terminalUIWrite(w, ui, uiIntentPath, value); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}
