package cli

import (
	"bytes"
	"strings"
	"testing"

	"helm/shared"
	"signal"
)

func TestDiagnosticRendererQuietOmitsInfoOnFlush(t *testing.T) {
	dr := DiagnosticRendererCreate(DiagnosticRendererConfig{ColorMode: ColorModeNone})
	dr.SetPresentation(DiagnosticPresentationQuiet)

	ctx := dr.Context()
	signal.SignalContextBuild(ctx, shared.SignalExecOK, "INFO").
		Payload(shared.PhasePayloadKey, shared.TargetExecutionPhase).
		Payload(shared.TargetPayloadKey, "demo").
		Emit()
	signal.SignalContextBuild(ctx, "ERR_DEMO", "ERROR").
		Payload(shared.MessagePayloadKey, "boom").
		Emit()

	var terminal bytes.Buffer
	dr.output = &terminal
	dr.Flush()

	out := terminal.String()
	if strings.Contains(out, "EXEC_OK") || strings.Contains(out, " OK ") {
		t.Fatalf("quiet flush should omit INFO execution signals: %s", out)
	}
	if !strings.Contains(out, "ERR_DEMO") && !strings.Contains(out, "boom") {
		t.Fatalf("quiet flush should keep ERROR signals: %s", out)
	}
}

func TestDiagnosticRendererFlushToIncludesFullDiagnostics(t *testing.T) {
	dr := DiagnosticRendererCreate(DiagnosticRendererConfig{ColorMode: ColorModeNone})
	dr.SetPresentation(DiagnosticPresentationSilent)

	ctx := dr.Context()
	signal.SignalContextBuild(ctx, shared.SignalExecOK, "INFO").
		Payload(shared.PhasePayloadKey, shared.TargetExecutionPhase).
		Payload(shared.TargetPayloadKey, "demo").
		Emit()

	var logBuffer bytes.Buffer
	dr.FlushTo(&logBuffer)

	out := StripTerminalEscapeSequences(logBuffer.String())
	if !strings.Contains(out, "EXEC_OK") {
		t.Fatalf("FlushTo should include INFO for transcript: %s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("FlushTo transcript must not contain ANSI escapes: %s", out)
	}
}
