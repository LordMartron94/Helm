package shared

import (
	"fmt"
	"foundation/location"
	"foundation/system"
	"signal"
	"signal/rendering"
	"splash"
	"strings"
)

func HelmCombineDetailHooks(hooks ...rendering.SignalDetailExtension) rendering.SignalDetailExtension {
	return func(renderer *splash.SPLASH_Rendering_TerminalRenderer, sig signal.Signal, baseIntent int) {
		for _, hook := range hooks {
			if hook != nil {
				hook(renderer, sig, baseIntent)
			}
		}
	}
}

// helmDiagnosticTabWidth must match tabWidth passed to syntaxa.LSTNodeLineSpanFromSource (IR + parse).
const helmDiagnosticTabWidth = 4

func HelmDiagnosticSquigglyDetailHook(metaIntent int) rendering.SignalDetailExtension {
	return func(renderer *splash.SPLASH_Rendering_TerminalRenderer, sig signal.Signal, intent int) {
		if !sig.HasLocation() {
			return
		}
		loc := sig.Location()
		if loc == nil || loc.Path() == "" {
			return
		}

		startLine, errSL := location.LocationCoordinateGetAs[int](*loc, "start_line")
		startCol, errSC := location.LocationCoordinateGetAs[int](*loc, "start_column")
		endCol, errEC := location.LocationCoordinateGetAs[int](*loc, "end_column")

		if errSL != nil || errSC != nil || startLine < 1 {
			return
		}

		content, err := system.FileReadAllRunes(loc.Path())
		if err != nil {
			return
		}
		lines := helmSplitLinesRunes(content)
		if startLine > len(lines) {
			return
		}

		lineRunes := lines[startLine-1]
		targetLine := helmExpandTabsForDisplay(lineRunes, helmDiagnosticTabWidth)

		markerWidth := 1
		if errEC == nil && endCol > startCol {
			markerWidth = endCol - startCol
		}

		lineNumber := fmt.Sprintf("%6d", startLine)
		lineGutter := lineNumber + " | "
		caretGutter := strings.Repeat(" ", len(lineNumber)) + " | "

		padding := helmVisualColumnToPadding(lineRunes, startCol, helmDiagnosticTabWidth)

		splash.SPLASH_Rendering_TerminalRendererIndent(renderer)
		splash.SPLASH_Rendering_TerminalRendererBufferColoredContent(renderer, lineGutter, metaIntent)
		splash.SPLASH_Rendering_TerminalRendererBufferContent(renderer, targetLine)
		splash.SPLASH_Rendering_TerminalRendererBufferLineBreak(renderer)

		splash.SPLASH_Rendering_TerminalRendererBufferColoredContent(renderer, caretGutter, metaIntent)

		if padding > 0 {
			splash.SPLASH_Rendering_TerminalRendererBufferContent(renderer, strings.Repeat(" ", padding))
		}

		marker := "^"
		if markerWidth > 1 {
			marker += strings.Repeat("~", markerWidth-1)
		}
		splash.SPLASH_Rendering_TerminalRendererBufferColoredContent(renderer, marker, intent)
		splash.SPLASH_Rendering_TerminalRendererBufferLineBreak(renderer)

		splash.SPLASH_Rendering_TerminalRendererDedent(renderer)
	}
}

func helmExpandTabsForDisplay(line []rune, tabWidth int) string {
	if tabWidth <= 0 {
		tabWidth = helmDiagnosticTabWidth
	}

	var builder strings.Builder
	builder.Grow(len(line) * 2)

	for _, r := range line {
		if r == '\t' {
			builder.WriteString(strings.Repeat(" ", tabWidth))
			continue
		}
		builder.WriteRune(r)
	}

	return builder.String()
}

func helmVisualColumnToPadding(line []rune, startColumn int, tabWidth int) int {
	if startColumn <= 1 {
		return 0
	}
	if tabWidth <= 0 {
		tabWidth = helmDiagnosticTabWidth
	}

	visual := 0
	for _, r := range line {
		if visual >= startColumn-1 {
			break
		}
		if r == '\t' {
			visual += tabWidth
			continue
		}
		visual++
	}

	return visual
}

func helmSplitLinesRunes(runes []rune) [][]rune {
	var lines [][]rune
	start := 0

	for i, r := range runes {
		if r == '\n' {
			lines = append(lines, runes[start:i])
			start = i + 1
		}
	}

	if start < len(runes) {
		lines = append(lines, runes[start:])
	}

	return lines
}

func HelmExecutionOutputDetailHook(metaIntent int) rendering.SignalDetailExtension {
	return HelmExecutionOutputDetailHookOmitBuffered(metaIntent, false)
}

// HelmExecutionOutputDetailHookOmitBuffered skips stdout/stderr blocks when output was streamed live.
func HelmExecutionOutputDetailHookOmitBuffered(metaIntent int, omitBufferedStdoutStderr bool) rendering.SignalDetailExtension {
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

		if !omitBufferedStdoutStderr {
			stdout, _ := signal.SignalPayloadGetAs[string](&sig, StdoutPayloadKey)
			stderr, _ := signal.SignalPayloadGetAs[string](&sig, StderrPayloadKey)

			if stdout != "" {
				helmRenderOutputBlock(renderer, "stdout", stdout, metaIntent, baseIntent)
			}
			if stderr != "" {
				helmRenderOutputBlock(renderer, "stderr", stderr, metaIntent, baseIntent)
			}
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
