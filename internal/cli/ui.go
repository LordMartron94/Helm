package cli

import (
	"io"
	"splash"
)

const uiIntentCount = 8

const (
	uiIntentPrompt = iota
	uiIntentAccent
	uiIntentHeading
	uiIntentName
	uiIntentMuted
	uiIntentLabel
	uiIntentError
	uiIntentPath
)

type TerminalUI struct {
	scratch *splash.SPLASH_Rendering_TerminalRenderer
}

func TerminalUICreate(mode ColorMode) *TerminalUI {
	paletteBuilder := splash.SPLASH_Rendering_TerminalPaletteBuilderCreate(uiIntentCount)
	paletteBuilder.Register(uiIntentPrompt, splash.SPLASH_Rendering_TerminalColorAnsi16_BrightCyan, splash.SPLASH_Rendering_TerminalTrueColor(86, 182, 194))
	paletteBuilder.Register(uiIntentAccent, splash.SPLASH_Rendering_TerminalColorAnsi16_BrightGreen, splash.SPLASH_Rendering_TerminalTrueColor(46, 204, 113))
	paletteBuilder.Register(uiIntentHeading, splash.SPLASH_Rendering_TerminalColorAnsi16_Cyan, splash.SPLASH_Rendering_TerminalTrueColor(52, 152, 219))
	paletteBuilder.Register(uiIntentName, splash.SPLASH_Rendering_TerminalColorAnsi16_BrightWhite, splash.SPLASH_Rendering_TerminalTrueColor(236, 240, 241))
	paletteBuilder.Register(uiIntentMuted, splash.SPLASH_Rendering_TerminalColorAnsi16_BrightBlack, splash.SPLASH_Rendering_TerminalTrueColor(127, 140, 141))
	paletteBuilder.Register(uiIntentLabel, splash.SPLASH_Rendering_TerminalColorAnsi16_Yellow, splash.SPLASH_Rendering_TerminalTrueColor(241, 196, 15))
	paletteBuilder.Register(uiIntentError, splash.SPLASH_Rendering_TerminalColorAnsi16_BrightRed, splash.SPLASH_Rendering_TerminalTrueColor(231, 76, 60))
	paletteBuilder.Register(uiIntentPath, splash.SPLASH_Rendering_TerminalColorAnsi16_Blue, splash.SPLASH_Rendering_TerminalTrueColor(52, 120, 246))
	palette := paletteBuilder.Build()

	return &TerminalUI{
		scratch: splash.SPLASH_Rendering_TerminalRendererCreate(colorModeToSplash(mode), palette),
	}
}

func colorModeToSplash(mode ColorMode) splash.SPLASH_Rendering_TerminalColorMode {
	switch mode {
	case ColorModeNone:
		return splash.SPLASH_Rendering_TerminalColorModeNone
	case ColorModeAnsi16:
		return splash.SPLASH_Rendering_TerminalColorModeAnsi16
	default:
		return splash.SPLASH_Rendering_TerminalColorModeTrueColor
	}
}

func terminalUIFormat(ui *TerminalUI, intent int, text string) string {
	splash.SPLASH_Rendering_TerminalRendererReset(ui.scratch)
	splash.SPLASH_Rendering_TerminalRendererBufferColoredContent(ui.scratch, text, intent)
	return splash.SPLASH_Rendering_TerminalRendererRender(ui.scratch)
}

func terminalUIWrite(w io.Writer, ui *TerminalUI, intent int, text string) error {
	_, err := io.WriteString(w, terminalUIFormat(ui, intent, text))
	return err
}

func terminalUIWritePrompt(w io.Writer, ui *TerminalUI) error {
	if err := terminalUIWrite(w, ui, uiIntentPrompt, "helm"); err != nil {
		return err
	}
	_, err := io.WriteString(w, "> ")
	return err
}

func terminalUIWriteLabelValue(w io.Writer, ui *TerminalUI, label, value string) error {
	if err := terminalUIWrite(w, ui, uiIntentMuted, label); err != nil {
		return err
	}
	return terminalUIWrite(w, ui, uiIntentPath, value)
}
