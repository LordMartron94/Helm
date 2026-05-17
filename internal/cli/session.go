package cli

import (
	"fmt"
	"helm/interpreter"
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
}

type SessionConfig struct {
	HelmFile  string
	LSpecPath string
	ColorMode ColorMode
}

func SessionCreate(config SessionConfig) (*Session, error) {
	interpreterInstance, err := interpreter.HelmInterpreterTryCreate(config.LSpecPath)
	if err != nil {
		return nil, err
	}

	renderer := DiagnosticRendererCreate(config.ColorMode, os.Stderr)

	result := interpreter.HelmInterpreterInterpretFile(interpreterInstance, config.HelmFile, renderer.Context())
	renderer.Flush()

	if result.Error != nil {
		interpreter.HelmInterpreterDestroy(interpreterInstance)
		return nil, result.Error
	}

	if !result.BuiltIR.Succeeded {
		interpreter.HelmInterpreterDestroy(interpreterInstance)
		return nil, fmt.Errorf("interpretation aborted due to semantic errors")
	}

	return &Session{
		HelmFile:    config.HelmFile,
		Interpreter: interpreterInstance,
		Result:      result,
		Catalog:     TargetCatalogBuild(result.BuiltIR),
		Renderer:    renderer,
		UI:          TerminalUICreate(config.ColorMode),
	}, nil
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
