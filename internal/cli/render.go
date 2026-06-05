package cli

import (
	"foundation/location"
	"helm/shared"
	"io"
	"os"
	"signal"
	"signal/rendering"
	"splash"
	"strconv"
)

const (
	intentCategoryInfo = iota
	intentCategoryWarning
	intentCategoryError
	intentDefault
	intentMeta
	intentExecSuccess
	intentExecSkipped
	intentExecFailed
	intentExecCache
	intentCount
)

type ColorMode string

const (
	ColorModeNone      ColorMode = "none"
	ColorModeAnsi16    ColorMode = "ansi16"
	ColorModeTrueColor ColorMode = "truecolor"
)

// DiagnosticPresentation controls post-run terminal diagnostic output.
type DiagnosticPresentation int

const (
	DiagnosticPresentationFull DiagnosticPresentation = iota
	DiagnosticPresentationQuiet
	DiagnosticPresentationSilent
)

type DiagnosticRenderer struct {
	dispatcher         *signal.SignalDispatcher
	renderer           *rendering.SignalRenderer
	logRenderer        *rendering.SignalRenderer
	errorRenderer      *rendering.SignalRenderer
	summaryRenderer    *splash.SPLASH_Rendering_TerminalRenderer
	logSummaryRenderer *splash.SPLASH_Rendering_TerminalRenderer
	execTally          *shared.HelmExecutionTally
	execIntents        shared.HelmExecutionRenderIntents
	ctx                *signal.SignalContext
	output             io.Writer
	presentation       DiagnosticPresentation
}

type DiagnosticRendererConfig struct {
	ColorMode ColorMode
	Output    io.Writer
}

func DiagnosticRendererCreate(config DiagnosticRendererConfig) *DiagnosticRenderer {
	output := config.Output
	if output == nil {
		output = os.Stderr
	}

	colorMode := config.ColorMode

	paletteBuilder := splash.SPLASH_Rendering_TerminalPaletteBuilderCreate(intentCount)
	paletteBuilder.Register(intentCategoryError, splash.SPLASH_Rendering_TerminalColorAnsi16_Red, splash.SPLASH_Rendering_TerminalTrueColor(231, 76, 60))
	paletteBuilder.Register(intentCategoryWarning, splash.SPLASH_Rendering_TerminalColorAnsi16_Yellow, splash.SPLASH_Rendering_TerminalTrueColor(241, 196, 15))
	paletteBuilder.Register(intentCategoryInfo, splash.SPLASH_Rendering_TerminalColorAnsi16_Cyan, splash.SPLASH_Rendering_TerminalTrueColor(52, 152, 219))
	paletteBuilder.Register(intentDefault, splash.SPLASH_Rendering_TerminalColorAnsi16_BrightBlack, splash.SPLASH_Rendering_TerminalTrueColor(127, 140, 141))
	paletteBuilder.Register(intentMeta, splash.SPLASH_Rendering_TerminalColorAnsi16_BrightBlack, splash.SPLASH_Rendering_TerminalTrueColor(127, 140, 141))
	paletteBuilder.Register(intentExecSuccess, splash.SPLASH_Rendering_TerminalColorAnsi16_BrightGreen, splash.SPLASH_Rendering_TerminalTrueColor(46, 204, 113))
	paletteBuilder.Register(intentExecSkipped, splash.SPLASH_Rendering_TerminalColorAnsi16_Yellow, splash.SPLASH_Rendering_TerminalTrueColor(241, 196, 15))
	paletteBuilder.Register(intentExecFailed, splash.SPLASH_Rendering_TerminalColorAnsi16_BrightRed, splash.SPLASH_Rendering_TerminalTrueColor(231, 76, 60))
	paletteBuilder.Register(intentExecCache, splash.SPLASH_Rendering_TerminalColorAnsi16_Cyan, splash.SPLASH_Rendering_TerminalTrueColor(52, 152, 219))
	palette := paletteBuilder.Build()

	splashMode := splash.SPLASH_Rendering_TerminalColorModeTrueColor
	switch colorMode {
	case ColorModeNone:
		splashMode = splash.SPLASH_Rendering_TerminalColorModeNone
	case ColorModeAnsi16:
		splashMode = splash.SPLASH_Rendering_TerminalColorModeAnsi16
	case ColorModeTrueColor:
		splashMode = splash.SPLASH_Rendering_TerminalColorModeTrueColor
	}

	terminalRenderer := splash.SPLASH_Rendering_TerminalRendererCreate(splashMode, palette)
	logSplashMode := splash.SPLASH_Rendering_TerminalColorModeNone
	logTerminalRenderer := splash.SPLASH_Rendering_TerminalRendererCreate(logSplashMode, palette)

	fileGrouping := rendering.GroupingConfiguration{
		ExtractKey: func(sig signal.Signal) string {
			if sig.HasLocation() && sig.Location().Path() != "" {
				return sig.Location().Path()
			}
			return "Global Diagnostics"
		},
		ResolveIntent: func(key string) int {
			switch key {
			case "INFO", "WARNING", "ERROR":
				return intentDefault
			default:
				return intentMeta
			}
		},
		PriorityOrder: []string{"Global Diagnostics"},
	}

	locationFormatter := func(loc *location.Location) string {
		startLine, errSL := location.LocationCoordinateGetAs[int](*loc, "start_line")
		startCol, errSC := location.LocationCoordinateGetAs[int](*loc, "start_column")
		if errSL == nil && errSC == nil && startLine > 0 {
			return " at " + strconv.Itoa(startLine) + ":" + strconv.Itoa(startCol)
		}
		return ""
	}

	execIntents := shared.HelmExecutionRenderIntents{
		Success: intentExecSuccess,
		Skipped: intentExecSkipped,
		Failed:  intentExecFailed,
		Cache:   intentExecCache,
		Meta:    intentMeta,
	}

	detailHook := shared.HelmCombineDetailHooks(
		shared.HelmDiagnosticSquigglyDetailHook(intentMeta),
		shared.HelmExecutionOutputDetailHookFailOnly(execIntents),
	)

	renderer := rendering.SignalRendererCreate(
		terminalRenderer,
		fileGrouping,
		locationFormatter,
		detailHook,
		intentMeta,
	)
	logRenderer := rendering.SignalRendererCreate(
		logTerminalRenderer,
		fileGrouping,
		locationFormatter,
		detailHook,
		intentMeta,
	)
	errorRenderer := rendering.SignalRendererCreate(
		terminalRenderer,
		fileGrouping,
		locationFormatter,
		detailHook,
		intentMeta,
	)

	manifest := signal.DiagnosticCategoryManifest{
		{Label: "INFO", Weight: 0},
		{Label: "WARNING", Weight: 10},
		{Label: "ERROR", Weight: 20},
	}
	dispatcher := signal.SignalDispatcherCreate(manifest)

	dr := &DiagnosticRenderer{
		dispatcher:         dispatcher,
		renderer:           renderer,
		logRenderer:        logRenderer,
		errorRenderer:      errorRenderer,
		summaryRenderer:    terminalRenderer,
		logSummaryRenderer: logTerminalRenderer,
		execTally:          &shared.HelmExecutionTally{},
		execIntents:        execIntents,
		ctx:                signal.SignalContextCreate(dispatcher),
		output:             output,
		presentation:       DiagnosticPresentationFull,
	}

	terminalSink := rendering.SignalRendererSinkGet(renderer)
	logSink := rendering.SignalRendererSinkGet(logRenderer)
	errorSink := rendering.SignalRendererSinkGet(errorRenderer)

	signal.SignalDispatcherRegisterSink(dispatcher, "cli", func(sig signal.Signal) {
		logSink(sig)
		category := sig.DiagnosticCategory()
		if category == "ERROR" || category == "WARNING" {
			errorSink(sig)
		}
		if dr.presentation == DiagnosticPresentationSilent {
			return
		}
		if dr.presentation == DiagnosticPresentationQuiet && category == "INFO" {
			return
		}
		terminalSink(sig)
	})

	signal.SignalDispatcherRegisterSink(dispatcher, "exec_tally", func(sig signal.Signal) {
		shared.HelmExecutionTallyRecord(dr.execTally, sig)
	})

	return dr
}

func (dr *DiagnosticRenderer) Context() *signal.SignalContext {
	return dr.ctx
}

func (dr *DiagnosticRenderer) SetPresentation(presentation DiagnosticPresentation) {
	dr.presentation = presentation
}

func (dr *DiagnosticRenderer) Presentation() DiagnosticPresentation {
	return dr.presentation
}

func (dr *DiagnosticRenderer) Flush() {
	dr.flushTo(dr.output, true)
}

func (dr *DiagnosticRenderer) FlushErrorsOnly() {
	if dr.errorRenderer == nil || dr.output == nil {
		return
	}
	_, _ = io.WriteString(dr.output, rendering.SignalRendererRender(dr.errorRenderer))
}

// FlushTo writes the full diagnostic render (including summary) to w regardless of presentation mode.
func (dr *DiagnosticRenderer) FlushTo(w io.Writer) {
	if w == nil || dr.logRenderer == nil {
		return
	}
	_, _ = io.WriteString(w, StripTerminalEscapeSequences(rendering.SignalRendererRender(dr.logRenderer)))
	if dr.logSummaryRenderer != nil && dr.execTally != nil && dr.execTally.HasExecutionSignals() {
		_, _ = io.WriteString(
			w,
			StripTerminalEscapeSequences(
				shared.HelmRenderExecutionSummary(dr.logSummaryRenderer, *dr.execTally, dr.execIntents),
			),
		)
	}
}

func (dr *DiagnosticRenderer) flushTo(w io.Writer, includeSummary bool) {
	if dr.renderer == nil || w == nil {
		return
	}
	if dr.presentation == DiagnosticPresentationSilent {
		return
	}
	_, _ = io.WriteString(w, rendering.SignalRendererRender(dr.renderer))
	if !includeSummary || dr.presentation != DiagnosticPresentationFull {
		return
	}
	if dr.summaryRenderer != nil && dr.execTally != nil {
		_, _ = io.WriteString(
			w,
			shared.HelmRenderExecutionSummary(dr.summaryRenderer, *dr.execTally, dr.execIntents),
		)
		*dr.execTally = shared.HelmExecutionTally{}
	}
}

func ResolveDefaultColorMode() ColorMode {
	if os.Getenv("NO_COLOR") != "" {
		return ColorModeNone
	}
	if stdoutIsTerminal() {
		return ColorModeTrueColor
	}
	if stderrIsTerminal() {
		return ColorModeAnsi16
	}
	return ColorModeNone
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func stderrIsTerminal() bool {
	info, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
