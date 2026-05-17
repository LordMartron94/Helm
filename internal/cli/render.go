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
	intentCount
)

type ColorMode string

const (
	ColorModeNone      ColorMode = "none"
	ColorModeAnsi16    ColorMode = "ansi16"
	ColorModeTrueColor ColorMode = "truecolor"
)

type DiagnosticRenderer struct {
	dispatcher *signal.SignalDispatcher
	renderer   *rendering.SignalRenderer
	ctx        *signal.SignalContext
	output     io.Writer
}

func DiagnosticRendererCreate(colorMode ColorMode, output io.Writer) *DiagnosticRenderer {
	if output == nil {
		output = os.Stderr
	}

	paletteBuilder := splash.SPLASH_Rendering_TerminalPaletteBuilderCreate(intentCount)
	paletteBuilder.Register(intentCategoryError, splash.SPLASH_Rendering_TerminalColorAnsi16_Red, splash.SPLASH_Rendering_TerminalTrueColor(231, 76, 60))
	paletteBuilder.Register(intentCategoryWarning, splash.SPLASH_Rendering_TerminalColorAnsi16_Yellow, splash.SPLASH_Rendering_TerminalTrueColor(241, 196, 15))
	paletteBuilder.Register(intentCategoryInfo, splash.SPLASH_Rendering_TerminalColorAnsi16_Cyan, splash.SPLASH_Rendering_TerminalTrueColor(52, 152, 219))
	paletteBuilder.Register(intentDefault, splash.SPLASH_Rendering_TerminalColorAnsi16_BrightBlack, splash.SPLASH_Rendering_TerminalTrueColor(127, 140, 141))
	paletteBuilder.Register(intentMeta, splash.SPLASH_Rendering_TerminalColorAnsi16_BrightBlack, splash.SPLASH_Rendering_TerminalTrueColor(127, 140, 141))
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

	detailHook := shared.HelmCombineDetailHooks(
		shared.HelmDiagnosticSquigglyDetailHook(intentMeta),
		shared.HelmExecutionOutputDetailHook(intentMeta),
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

	return &DiagnosticRenderer{
		dispatcher: dispatcher,
		renderer:   renderer,
		ctx:        signal.SignalContextCreate(dispatcher),
		output:     output,
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
