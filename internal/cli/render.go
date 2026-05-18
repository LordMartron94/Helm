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

type DiagnosticRenderer struct {
	dispatcher      *signal.SignalDispatcher
	renderer        *rendering.SignalRenderer
	summaryRenderer *splash.SPLASH_Rendering_TerminalRenderer
	execTally       *shared.HelmExecutionTally
	execIntents     shared.HelmExecutionRenderIntents
	ctx             *signal.SignalContext
	output          io.Writer
}

type DiagnosticRendererConfig struct {
	ColorMode       ColorMode
	Output          io.Writer
	StreamRunOutput *bool
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

	omitBufferedExecOutput := false
	if config.StreamRunOutput != nil {
		omitBufferedExecOutput = *config.StreamRunOutput
	}

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
		shared.HelmExecutionOutputDetailHookOmitBuffered(execIntents, omitBufferedExecOutput),
	)

	renderer := rendering.SignalRendererCreate(
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
	signal.SignalDispatcherRegisterSink(dispatcher, "cli", rendering.SignalRendererSinkGet(renderer))

	execTally := &shared.HelmExecutionTally{}
	signal.SignalDispatcherRegisterSink(dispatcher, "exec_tally", func(sig signal.Signal) {
		shared.HelmExecutionTallyRecord(execTally, sig)
	})

	return &DiagnosticRenderer{
		dispatcher:      dispatcher,
		renderer:        renderer,
		summaryRenderer: terminalRenderer,
		execTally:       execTally,
		execIntents:     execIntents,
		ctx:             signal.SignalContextCreate(dispatcher),
		output:          output,
	}
}

func (dr *DiagnosticRenderer) Context() *signal.SignalContext {
	return dr.ctx
}

func (dr *DiagnosticRenderer) Flush() {
	if dr.renderer == nil {
		return
	}
	_, _ = io.WriteString(dr.output, rendering.SignalRendererRender(dr.renderer))
	if dr.summaryRenderer != nil && dr.execTally != nil {
		_, _ = io.WriteString(
			dr.output,
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
