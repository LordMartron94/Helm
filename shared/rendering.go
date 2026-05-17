package shared

import (
	"fmt"
	"signal"
	"signal/rendering"
	"splash"
	"strings"
)

func HelmExecutionOutputDetailHook(metaIntent int) rendering.SignalDetailExtension {
	return func(renderer *splash.SPLASH_Rendering_TerminalRenderer, sig signal.Signal, baseIntent int) {
		id := sig.ID()
		if id == SignalExecSkipped {
			phase, err := signal.SignalPayloadGetAs[string](&sig, PhasePayloadKey)
			if err != nil || phase != TargetExecutionPhase {
				return
			}
			target, _ := signal.SignalPayloadGetAs[string](&sig, TargetPayloadKey)
			reason, _ := signal.SignalPayloadGetAs[string](&sig, ReasonPayloadKey)
			splash.SPLASH_Rendering_TerminalRendererIndent(renderer)
			splash.SPLASH_Rendering_TerminalRendererBufferColoredContent(renderer, "skipped:", metaIntent)
			if target != "" {
				splash.SPLASH_Rendering_TerminalRendererBufferContent(renderer, fmt.Sprintf(" %s", target))
			}
			if reason != "" {
				splash.SPLASH_Rendering_TerminalRendererBufferContent(renderer, fmt.Sprintf(" (%s)", reason))
			}
			splash.SPLASH_Rendering_TerminalRendererBufferLineBreak(renderer)
			splash.SPLASH_Rendering_TerminalRendererDedent(renderer)
			return
		}

		if id == SignalCacheUpdated {
			phase, err := signal.SignalPayloadGetAs[string](&sig, PhasePayloadKey)
			if err != nil || phase != TargetExecutionPhase {
				return
			}
			target, _ := signal.SignalPayloadGetAs[string](&sig, TargetPayloadKey)
			splash.SPLASH_Rendering_TerminalRendererIndent(renderer)
			splash.SPLASH_Rendering_TerminalRendererBufferColoredContent(renderer, "cache updated:", metaIntent)
			if target != "" {
				splash.SPLASH_Rendering_TerminalRendererBufferContent(renderer, fmt.Sprintf(" %s", target))
			}
			splash.SPLASH_Rendering_TerminalRendererBufferLineBreak(renderer)
			splash.SPLASH_Rendering_TerminalRendererDedent(renderer)
			return
		}

		if id != SignalExecOK && id != SignalExecFail {
			return
		}

		phase, err := signal.SignalPayloadGetAs[string](&sig, PhasePayloadKey)
		if err != nil || phase != TargetExecutionPhase {
			return
		}

		stdout, _ := signal.SignalPayloadGetAs[string](&sig, StdoutPayloadKey)
		stderr, _ := signal.SignalPayloadGetAs[string](&sig, StderrPayloadKey)

		if stdout != "" {
			helmRenderOutputBlock(renderer, "stdout", stdout, metaIntent, baseIntent)
		}
		if stderr != "" {
			helmRenderOutputBlock(renderer, "stderr", stderr, metaIntent, baseIntent)
		}

		if id == SignalExecFail {
			exitCode, err := signal.SignalPayloadGetAs[int](&sig, ExitCodePayloadKey)
			if err == nil {
				splash.SPLASH_Rendering_TerminalRendererIndent(renderer)
				splash.SPLASH_Rendering_TerminalRendererBufferColoredContent(renderer, "exit_code:", metaIntent)
				splash.SPLASH_Rendering_TerminalRendererBufferContent(renderer, fmt.Sprintf(" %d", exitCode))
				splash.SPLASH_Rendering_TerminalRendererBufferLineBreak(renderer)
				splash.SPLASH_Rendering_TerminalRendererDedent(renderer)
			}
		}
	}
}

func helmRenderOutputBlock(
	renderer *splash.SPLASH_Rendering_TerminalRenderer,
	label string,
	body string,
	labelIntent int,
	bodyIntent int,
) {
	splash.SPLASH_Rendering_TerminalRendererIndent(renderer)
	splash.SPLASH_Rendering_TerminalRendererBufferColoredContent(renderer, label+":", labelIntent)
	splash.SPLASH_Rendering_TerminalRendererBufferLineBreak(renderer)

	trimmed := strings.TrimRight(body, "\n")
	if trimmed == "" {
		splash.SPLASH_Rendering_TerminalRendererDedent(renderer)
		return
	}

	lines := strings.Split(trimmed, "\n")
	for _, line := range lines {
		splash.SPLASH_Rendering_TerminalRendererIndent(renderer)
		splash.SPLASH_Rendering_TerminalRendererBufferColoredContent(renderer, "  ", labelIntent)
		splash.SPLASH_Rendering_TerminalRendererBufferColoredContent(renderer, line, bodyIntent)
		splash.SPLASH_Rendering_TerminalRendererBufferLineBreak(renderer)
		splash.SPLASH_Rendering_TerminalRendererDedent(renderer)
	}

	splash.SPLASH_Rendering_TerminalRendererDedent(renderer)
}
