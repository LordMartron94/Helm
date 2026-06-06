package cli

import (
	"helm/interpreter"
	"io"
	"os"
	"path/filepath"
)

type Session struct {
	HelmFile    string
	Interpreter *interpreter.HelmInterpreter
	Result      interpreter.HelmInterpreterInterpretationResult
	Catalog     TargetCatalog
	Renderer    *DiagnosticRenderer
	UI          *TerminalUI
	// StreamRunOutput streams subprocess stdout/stderr during runs instead of only after completion.
	StreamRunOutput bool
}

type SessionConfig struct {
	HelmFile         string
	LSpecPath        string
	ColorMode        ColorMode
	DiagnosticOutput io.Writer
	// StreamRunOutput defaults to true when unset at the call site (see SessionCreate).
	StreamRunOutput *bool
}

func SessionCreate(config SessionConfig) (*Session, error) {
	interpreterInstance, err := interpreter.HelmInterpreterTryCreate(config.LSpecPath)
	if err != nil {
		return nil, err
	}

	session := &Session{
		HelmFile:        config.HelmFile,
		Interpreter:     interpreterInstance,
		UI:              TerminalUICreate(config.ColorMode),
		StreamRunOutput: true,
	}
	if config.StreamRunOutput != nil {
		session.StreamRunOutput = *config.StreamRunOutput
	}

	diagnosticOutput := config.DiagnosticOutput
	if diagnosticOutput == nil {
		diagnosticOutput = os.Stderr
	}

	session.Renderer = DiagnosticRendererCreate(DiagnosticRendererConfig{
		ColorMode: config.ColorMode,
		Output:    diagnosticOutput,
	})

	mergedIR, loadErr := interpreter.HelmInterpreterLoadWorkspace(
		interpreterInstance,
		config.HelmFile,
		session.Renderer.Context(),
	)
	session.Renderer.Flush()

	if loadErr != nil {
		interpreter.HelmInterpreterDestroy(interpreterInstance)
		return nil, loadErr
	}

	session.Result = interpreter.HelmInterpreterInterpretationResultFromIR(
		interpreterInstance,
		config.HelmFile,
		mergedIR,
	)
	session.Catalog = TargetCatalogBuild(mergedIR)
	return session, nil
}

func SessionDestroy(session *Session) {
	if session == nil {
		return
	}
	if session.Interpreter != nil {
		interpreter.HelmInterpreterDestroy(session.Interpreter)
	}
}

func (session *Session) FlushDiagnostics() {
	session.Renderer.Flush()
}

func (session *Session) SourceDirectory() string {
	return session.Result.BuiltIR.SourceDirectory
}

func (session *Session) CacheDirectory() string {
	return filepath.Join(session.SourceDirectory(), ".helm", "cache")
}
