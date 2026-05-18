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

// HelmExecutionRenderIntents maps execution outcome lines to palette slots on the CLI renderer.
type HelmExecutionRenderIntents struct {
	Success int
	Skipped int
	Failed  int
	Cache   int
	Meta    int
}

// HelmExecutionTally counts target-execution signals emitted during a run.
type HelmExecutionTally struct {
	OK      int
	Failed  int
	Skipped int
	Cache   int
}

func HelmExecutionTallyRecord(tally *HelmExecutionTally, sig signal.Signal) {
	if tally == nil {
		return
	}

	phase, err := signal.SignalPayloadGetAs[string](&sig, PhasePayloadKey)
	if err != nil || phase != TargetExecutionPhase {
		return
	}

	switch sig.ID() {
	case SignalExecOK:
		tally.OK++
	case SignalExecFail:
		tally.Failed++
	case SignalExecSkipped:
		tally.Skipped++
	case SignalCacheUpdated:
		tally.Cache++
	}
}

func (tally HelmExecutionTally) HasExecutionSignals() bool {
	return tally.OK+tally.Failed+tally.Skipped+tally.Cache > 0
}

// HelmRenderExecutionSummary formats a colored footer with execution outcome counts.
func HelmRenderExecutionSummary(
	renderer *splash.SPLASH_Rendering_TerminalRenderer,
	tally HelmExecutionTally,
	intents HelmExecutionRenderIntents,
) string {
	if !tally.HasExecutionSignals() {
		return ""
	}

	splash.SPLASH_Rendering_TerminalRendererReset(renderer)
	splash.SPLASH_Rendering_TerminalRendererBufferLineBreak(renderer)
	splash.SPLASH_Rendering_TerminalRendererBufferColoredContent(
		renderer,
		"=== Execution summary ===",
		intents.Meta,
	)
	splash.SPLASH_Rendering_TerminalRendererBufferLineBreak(renderer)
	splash.SPLASH_Rendering_TerminalRendererIndent(renderer)

	helmRenderExecutionSummaryCount(renderer, "OK", tally.OK, intents.Success)
	helmRenderExecutionSummaryCount(renderer, "FAILED", tally.Failed, intents.Failed)
	helmRenderExecutionSummaryCount(renderer, "SKIPPED", tally.Skipped, intents.Skipped)
	helmRenderExecutionSummaryCount(renderer, "CACHE", tally.Cache, intents.Cache)

	splash.SPLASH_Rendering_TerminalRendererDedent(renderer)
	splash.SPLASH_Rendering_TerminalRendererBufferLineBreak(renderer)

	return splash.SPLASH_Rendering_TerminalRendererRender(renderer)
}

func helmRenderExecutionSummaryCount(
	renderer *splash.SPLASH_Rendering_TerminalRenderer,
	label string,
	count int,
	intent int,
) {
	if count == 0 {
		return
	}

	splash.SPLASH_Rendering_TerminalRendererBufferColoredContent(renderer, label, intent)
	splash.SPLASH_Rendering_TerminalRendererBufferContent(renderer, fmt.Sprintf(": %d", count))
	splash.SPLASH_Rendering_TerminalRendererBufferLineBreak(renderer)
}

func HelmExecutionOutputDetailHook(intents HelmExecutionRenderIntents) rendering.SignalDetailExtension {
	return HelmExecutionOutputDetailHookOmitBuffered(intents, false)
}

// HelmExecutionOutputDetailHookOmitBuffered skips stdout/stderr blocks when output was streamed live.
func HelmExecutionOutputDetailHookOmitBuffered(
	intents HelmExecutionRenderIntents,
	omitBufferedStdoutStderr bool,
) rendering.SignalDetailExtension {
	return func(renderer *splash.SPLASH_Rendering_TerminalRenderer, sig signal.Signal, baseIntent int) {
		id := sig.ID()
		if id == SignalExecSkipped {
			phase, err := signal.SignalPayloadGetAs[string](&sig, PhasePayloadKey)
			if err != nil || phase != TargetExecutionPhase {
				return
			}
			target, _ := signal.SignalPayloadGetAs[string](&sig, TargetPayloadKey)
			reason, _ := signal.SignalPayloadGetAs[string](&sig, ReasonPayloadKey)
			helmRenderExecutionStatus(renderer, "SKIPPED", intents.Skipped, intents.Meta, target, reason)
			return
		}

		if id == SignalCacheUpdated {
			phase, err := signal.SignalPayloadGetAs[string](&sig, PhasePayloadKey)
			if err != nil || phase != TargetExecutionPhase {
				return
			}
			target, _ := signal.SignalPayloadGetAs[string](&sig, TargetPayloadKey)
			helmRenderExecutionStatus(renderer, "CACHE", intents.Cache, intents.Meta, target, "")
			return
		}

		if id != SignalExecOK && id != SignalExecFail {
			return
		}

		phase, err := signal.SignalPayloadGetAs[string](&sig, PhasePayloadKey)
		if err != nil || phase != TargetExecutionPhase {
			return
		}

		target, _ := signal.SignalPayloadGetAs[string](&sig, TargetPayloadKey)
		command, _ := signal.SignalPayloadGetAs[string](&sig, CommandPayloadKey)

		switch id {
		case SignalExecOK:
			helmRenderExecutionStatus(renderer, "OK", intents.Success, intents.Meta, target, command)
		case SignalExecFail:
			helmRenderExecutionStatus(renderer, "FAILED", intents.Failed, intents.Meta, target, command)
		}

		if !omitBufferedStdoutStderr {
			stdout, _ := signal.SignalPayloadGetAs[string](&sig, StdoutPayloadKey)
			stderr, _ := signal.SignalPayloadGetAs[string](&sig, StderrPayloadKey)

			if stdout != "" {
				helmRenderOutputBlock(renderer, "stdout", stdout, intents.Meta, intents.Success)
			}
			if stderr != "" {
				helmRenderOutputBlock(renderer, "stderr", stderr, intents.Meta, intents.Failed)
			}
		}

		if id == SignalExecFail {
			exitCode, err := signal.SignalPayloadGetAs[int](&sig, ExitCodePayloadKey)
			if err == nil {
				splash.SPLASH_Rendering_TerminalRendererIndent(renderer)
				splash.SPLASH_Rendering_TerminalRendererBufferColoredContent(renderer, "exit code:", intents.Failed)
				splash.SPLASH_Rendering_TerminalRendererBufferContent(renderer, fmt.Sprintf(" %d", exitCode))
				splash.SPLASH_Rendering_TerminalRendererBufferLineBreak(renderer)
				splash.SPLASH_Rendering_TerminalRendererDedent(renderer)
			}
		}
	}
}

func helmRenderExecutionStatus(
	renderer *splash.SPLASH_Rendering_TerminalRenderer,
	statusLabel string,
	statusIntent int,
	metaIntent int,
	target string,
	detail string,
) {
	splash.SPLASH_Rendering_TerminalRendererIndent(renderer)
	splash.SPLASH_Rendering_TerminalRendererBufferColoredContent(renderer, statusLabel, statusIntent)
	if target != "" {
		splash.SPLASH_Rendering_TerminalRendererBufferContent(renderer, "  ")
		splash.SPLASH_Rendering_TerminalRendererBufferContent(renderer, target)
	}
	splash.SPLASH_Rendering_TerminalRendererBufferLineBreak(renderer)

	if detail != "" {
		splash.SPLASH_Rendering_TerminalRendererIndent(renderer)
		splash.SPLASH_Rendering_TerminalRendererBufferColoredContent(renderer, "  ", metaIntent)
		splash.SPLASH_Rendering_TerminalRendererBufferContent(renderer, detail)
		splash.SPLASH_Rendering_TerminalRendererBufferLineBreak(renderer)
		splash.SPLASH_Rendering_TerminalRendererDedent(renderer)
	}

	splash.SPLASH_Rendering_TerminalRendererDedent(renderer)
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
