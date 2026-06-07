package shared

import (
	"strings"
	"testing"

	"signal"
	"splash"
)

func TestHelmExecutionOutputDetailHookFailOnlyOmitsSuccessOutput(t *testing.T) {
	renderer, intents := helmTestTerminalRenderer(t)
	hook := HelmExecutionOutputDetailHookFailOnly(intents)

	sig := helmTestBuildExecSignal(SignalExecOK, "stdout payload", "stderr payload")
	hook(renderer, sig, intents.Meta)

	output := splash.SPLASH_Rendering_TerminalRendererRender(renderer)
	if strings.Contains(output, "stdout payload") || strings.Contains(output, "stderr payload") {
		t.Fatalf("expected success signal to omit stdout/stderr, got: %s", output)
	}
}

func TestHelmExecutionOutputDetailHookFailOnlyRendersFailureMessage(t *testing.T) {
	renderer, intents := helmTestTerminalRenderer(t)
	hook := HelmExecutionOutputDetailHookFailOnly(intents)

	sig := helmTestBuildExecFailSignal("signal: segmentation fault (core dumped)")
	hook(renderer, sig, intents.Meta)

	output := splash.SPLASH_Rendering_TerminalRendererRender(renderer)
	if !strings.Contains(output, "segmentation fault") {
		t.Fatalf("expected failure message in render, got: %s", output)
	}
}

func TestHelmExecutionOutputDetailHookFailOnlyIncludesFailureOutput(t *testing.T) {
	renderer, intents := helmTestTerminalRenderer(t)
	hook := HelmExecutionOutputDetailHookFailOnly(intents)

	sig := helmTestBuildExecSignal(SignalExecFail, "stdout payload", "stderr payload")
	hook(renderer, sig, intents.Meta)

	output := splash.SPLASH_Rendering_TerminalRendererRender(renderer)
	if !strings.Contains(output, "stdout payload") {
		t.Fatalf("expected failure stdout in render, got: %s", output)
	}
	if !strings.Contains(output, "stderr payload") {
		t.Fatalf("expected failure stderr in render, got: %s", output)
	}
}

func helmTestTerminalRenderer(t *testing.T) (*splash.SPLASH_Rendering_TerminalRenderer, HelmExecutionRenderIntents) {
	t.Helper()

	paletteBuilder := splash.SPLASH_Rendering_TerminalPaletteBuilderCreate(8)
	paletteBuilder.Register(0, splash.SPLASH_Rendering_TerminalColorAnsi16_Red, splash.SPLASH_Rendering_TerminalTrueColor(231, 76, 60))
	paletteBuilder.Register(1, splash.SPLASH_Rendering_TerminalColorAnsi16_Yellow, splash.SPLASH_Rendering_TerminalTrueColor(241, 196, 15))
	paletteBuilder.Register(2, splash.SPLASH_Rendering_TerminalColorAnsi16_Cyan, splash.SPLASH_Rendering_TerminalTrueColor(52, 152, 219))
	paletteBuilder.Register(3, splash.SPLASH_Rendering_TerminalColorAnsi16_BrightBlack, splash.SPLASH_Rendering_TerminalTrueColor(127, 140, 141))
	paletteBuilder.Register(4, splash.SPLASH_Rendering_TerminalColorAnsi16_BrightGreen, splash.SPLASH_Rendering_TerminalTrueColor(46, 204, 113))
	paletteBuilder.Register(5, splash.SPLASH_Rendering_TerminalColorAnsi16_BrightRed, splash.SPLASH_Rendering_TerminalTrueColor(231, 76, 60))
	palette := paletteBuilder.Build()

	renderer := splash.SPLASH_Rendering_TerminalRendererCreate(
		splash.SPLASH_Rendering_TerminalColorModeNone,
		palette,
	)

	intents := HelmExecutionRenderIntents{
		Success: 4,
		Failed:  5,
		Meta:    3,
	}
	return renderer, intents
}

func helmTestBuildExecFailSignal(message string) signal.Signal {
	manifest := signal.DiagnosticCategoryManifest{
		{Label: "INFO", Weight: 0},
		{Label: "ERROR", Weight: 10},
	}
	ctx := signal.SignalContextCreate(signal.SignalDispatcherCreate(manifest))
	return signal.SignalContextBuild(ctx, SignalExecFail, "ERROR").
		Payload(PhasePayloadKey, TargetExecutionPhase).
		Payload(TargetPayloadKey, "run_tests").
		Payload(CommandPayloadKey, "build/bin/testbed").
		Payload(MessagePayloadKey, message).
		Payload(ExitCodePayloadKey, -1).
		Payload(DurationNSPayloadKey, int64(141_000_000)).
		Build()
}

func helmTestBuildExecSignal(id string, stdout string, stderr string) signal.Signal {
	manifest := signal.DiagnosticCategoryManifest{
		{Label: "INFO", Weight: 0},
		{Label: "ERROR", Weight: 10},
	}
	ctx := signal.SignalContextCreate(signal.SignalDispatcherCreate(manifest))
	return signal.SignalContextBuild(ctx, id, "INFO").
		Payload(PhasePayloadKey, TargetExecutionPhase).
		Payload(TargetPayloadKey, "demo").
		Payload(CommandPayloadKey, "echo demo").
		Payload(StdoutPayloadKey, stdout).
		Payload(StderrPayloadKey, stderr).
		Payload(DurationNSPayloadKey, int64(1)).
		Build()
}
