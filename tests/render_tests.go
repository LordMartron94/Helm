package tests

import (
	"fmt"
	"foundation/location"
	"foundation/system"
	"helm/interpreter"
	"helm/shared"
	"path/filepath"
	"signal"
	"signal/rendering"
	"splash"
	"strings"
	"testing"
)

const (
	IntentCategoryInfo = iota
	IntentCategoryWarning
	IntentCategoryError
	IntentDefault
	IntentMeta
	intentCount
)

var defaultRenderingIntentMap = map[string]int{
	"INFO":    IntentCategoryInfo,
	"WARNING": IntentCategoryWarning,
	"ERROR":   IntentCategoryError,
}

var categoryGroupingStrategy = rendering.GroupingConfiguration{
	ExtractKey: func(sig signal.Signal) string {
		return string(sig.DiagnosticCategory())
	},
	ResolveIntent: func(groupKey string) int {
		intent, exists := defaultRenderingIntentMap[groupKey]
		if !exists {
			return IntentDefault
		}
		return intent
	},
	PriorityOrder: []string{"ERROR", "WARNING", "INFO"},
}

var fileGroupingStrategy = rendering.GroupingConfiguration{
	ExtractKey: func(sig signal.Signal) string {
		if sig.HasLocation() && sig.Location().Path() != "" {
			return sig.Location().Path()
		}
		return "Global Diagnostics"
	},
	ResolveIntent: func(key string) int {
		if intent, exists := defaultRenderingIntentMap[key]; exists {
			return intent
		}

		return IntentMeta
	},
	PriorityOrder: []string{"Global Diagnostics"},
}

var squigglyRenderer = func(renderer *splash.SPLASH_Rendering_TerminalRenderer, sig signal.Signal, intent int) {
	if !sig.HasLocation() {
		return // Nothing to draw
	}
	loc := sig.Location()
	if loc == nil || loc.Path() == "" {
		return
	}

	// 1. Extract exact coordinates safely
	startLine, errSL := location.LocationCoordinateGetAs[int](*loc, "start_line")
	startCol, errSC := location.LocationCoordinateGetAs[int](*loc, "start_column")
	endCol, errEC := location.LocationCoordinateGetAs[int](*loc, "end_column")

	if errSL != nil || errSC != nil || startLine < 1 {
		return // Not enough data to draw a snippet
	}

	// 2. Read the source line (Simplified for example. In reality, you'd want
	// to cache file reads or use a line-scanner to avoid reading the whole file every time).
	content, err := system.FileReadAllRunes(loc.Path())
	if err != nil {
		return
	}
	lines := splitLinesRunes(content) // Using your existing helper
	if startLine > len(lines) {
		return
	}

	targetLine := string(lines[startLine-1])

	// 3. Calculate squiggly width
	width := 1
	if errEC == nil && endCol > startCol {
		width = endCol - startCol
	}

	// 4. Draw using Splash Primitives
	splash.SPLASH_Rendering_TerminalRendererIndent(renderer)

	// Draw Line Number and Code: "  15 | target syntax_error() {"
	splash.SPLASH_Rendering_TerminalRendererBufferColoredContent(renderer, fmt.Sprintf("%4d | ", startLine), IntentMeta)
	splash.SPLASH_Rendering_TerminalRendererBufferContent(renderer, targetLine)
	splash.SPLASH_Rendering_TerminalRendererBufferLineBreak(renderer)

	// Draw the Squigglies:       "       ^^^^^^"
	splash.SPLASH_Rendering_TerminalRendererBufferColoredContent(renderer, "     | ", IntentMeta)

	// Pad to the column (assuming 1-based column indexing)
	padding := startCol - 1
	if padding > 0 {
		splash.SPLASH_Rendering_TerminalRendererBufferContent(renderer, strings.Repeat(" ", padding))
	}

	// Draw the caret/squiggles in the exact color of the error/warning
	marker := "^"
	if width > 1 {
		marker += strings.Repeat("~", width-1)
	}
	splash.SPLASH_Rendering_TerminalRendererBufferColoredContent(renderer, marker, intent)
	splash.SPLASH_Rendering_TerminalRendererBufferLineBreak(renderer)

	splash.SPLASH_Rendering_TerminalRendererDedent(renderer)
}

func splitLinesRunes(runes []rune) [][]rune {
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

func HelmRenderTestSyntax(t *testing.T) {
	var collected []helmSyntaxDiagnostic

	executeVisualDiagnosticHarness(t, "bad_syntax.helm", "",
		// Collector Closure
		func(sig signal.Signal) error {
			diag, err := helmSyntaxDiagnosticFromSignal(sig)
			if err != nil {
				return err
			}
			collected = append(collected, diag)
			return nil
		},
		// Validation Closure
		func() error {
			if ok, reason := helmBadSyntaxDiagnosticsMatch(collected); !ok {
				return fmt.Errorf(reason)
			}
			return nil
		},
	)
}

func HelmRenderTestSemantics(t *testing.T) {
	var collected []helmSemanticDiagnostic

	executeVisualDiagnosticHarness(t, "bad_semantics.helm", "",
		// Collector Closure
		func(sig signal.Signal) error {
			phase, err := signal.SignalPayloadGetAs[string](&sig, "phase")
			// Ignore syntax signals bleeding into the semantics test
			if err != nil || phase != "semantics" {
				return nil
			}

			diag, err := helmSemanticDiagnosticFromSignal(sig)
			if err != nil {
				return err
			}
			collected = append(collected, diag)
			return nil
		},
		// Validation Closure
		func() error {
			if ok, reason := helmBadSemanticsDiagnosticsMatch(collected); !ok {
				return fmt.Errorf(reason)
			}
			return nil
		},
	)
}

func HelmRenderTestGraph(t *testing.T) {
	var collected []string

	executeVisualDiagnosticHarness(t, "bad_dag_cycle.helm", "a", // Trigger the DAG pipeline for target "a"
		// Collector Closure
		func(sig signal.Signal) error {
			phase, err := signal.SignalPayloadGetAs[string](&sig, shared.PhasePayloadKey)
			if err != nil || phase != shared.GraphResolutionPhase {
				return nil
			}

			msg, err := signal.SignalPayloadGetAs[string](&sig, shared.MessagePayloadKey)
			if err != nil {
				return err
			}
			collected = append(collected, msg)
			return nil
		},
		// Validation Closure
		func() error {
			if len(collected) == 0 {
				return fmt.Errorf("expected graph resolution errors, got none")
			}

			foundCycle := false
			for _, msg := range collected {
				if strings.Contains(strings.ToLower(msg), "cycle") {
					foundCycle = true
					break
				}
			}

			if !foundCycle {
				return fmt.Errorf("expected cycle error, got: %v", collected)
			}
			return nil
		},
	)
}

func executeVisualDiagnosticHarness(
	t *testing.T,
	targetFileName string,
	targetToResolve string, // ADDED: "" means skip DAG resolution
	onSignal func(sig signal.Signal) error,
	validate func() error,
) {
	paletteBuilder := splash.SPLASH_Rendering_TerminalPaletteBuilderCreate(int(intentCount))
	paletteBuilder.Register(IntentCategoryError, splash.SPLASH_Rendering_TerminalColorAnsi16_Red, splash.SPLASH_Rendering_TerminalTrueColor(231, 76, 60))
	paletteBuilder.Register(IntentCategoryWarning, splash.SPLASH_Rendering_TerminalColorAnsi16_Yellow, splash.SPLASH_Rendering_TerminalTrueColor(241, 196, 15))
	paletteBuilder.Register(IntentCategoryInfo, splash.SPLASH_Rendering_TerminalColorAnsi16_Cyan, splash.SPLASH_Rendering_TerminalTrueColor(52, 152, 219))
	paletteBuilder.Register(IntentDefault, splash.SPLASH_Rendering_TerminalColorAnsi16_BrightBlack, splash.SPLASH_Rendering_TerminalTrueColor(127, 140, 141))
	paletteBuilder.Register(IntentMeta, splash.SPLASH_Rendering_TerminalColorAnsi16_BrightBlack, splash.SPLASH_Rendering_TerminalTrueColor(127, 140, 141))

	palette := paletteBuilder.Build()

	splashNone := splash.SPLASH_Rendering_TerminalRendererCreate(splash.SPLASH_Rendering_TerminalColorModeNone, palette)
	splashANSI := splash.SPLASH_Rendering_TerminalRendererCreate(splash.SPLASH_Rendering_TerminalColorModeAnsi16, palette)
	splashTrue := splash.SPLASH_Rendering_TerminalRendererCreate(splash.SPLASH_Rendering_TerminalColorModeTrueColor, palette)

	clientLocationFormatter := func(loc *location.Location) string {
		startLine, errSL := location.LocationCoordinateGetAs[int](*loc, "start_line")
		startCol, errSC := location.LocationCoordinateGetAs[int](*loc, "start_column")
		if errSL == nil && errSC == nil && startLine > 0 {
			return fmt.Sprintf(" at %d:%d", startLine, startCol)
		}
		return ""
	}

	rendererNone := rendering.SignalRendererCreate(splashNone, fileGroupingStrategy, clientLocationFormatter, squigglyRenderer, IntentMeta)
	rendererANSI := rendering.SignalRendererCreate(splashANSI, fileGroupingStrategy, clientLocationFormatter, squigglyRenderer, IntentMeta)
	rendererTrue := rendering.SignalRendererCreate(splashTrue, fileGroupingStrategy, clientLocationFormatter, squigglyRenderer, IntentMeta)

	manifest := signal.DiagnosticCategoryManifest{
		{Label: "INFO", Weight: 0},
		{Label: "WARNING", Weight: 10},
		{Label: "ERROR", Weight: 20},
	}
	dispatcher := signal.SignalDispatcherCreate(manifest)

	signal.SignalDispatcherRegisterSink(dispatcher, "sink_none", rendering.SignalRendererSinkGet(rendererNone))
	signal.SignalDispatcherRegisterSink(dispatcher, "sink_ansi", rendering.SignalRendererSinkGet(rendererANSI))
	signal.SignalDispatcherRegisterSink(dispatcher, "sink_true", rendering.SignalRendererSinkGet(rendererTrue))

	var collectErr error
	if onSignal != nil {
		signal.SignalDispatcherRegisterSink(dispatcher, "collect", func(sig signal.Signal) {
			if err := onSignal(sig); err != nil && collectErr == nil {
				collectErr = err
			}
		})
	}

	specPath, err := helmLSpecPathResolve()
	if err != nil {
		t.Fatalf("helm test setup failed (resolve spec): %v", err)
	}
	casesDir, err := helmTestsCasesDirResolve()
	if err != nil {
		t.Fatalf("helm test setup failed (resolve cases dir): %v", err)
	}

	sharedHelm, err := interpreter.HelmInterpreterTryCreate(specPath)
	if err != nil || sharedHelm == nil {
		t.Fatalf("interpreter creation failed: %v", err)
	}
	defer interpreter.HelmInterpreterDestroy(sharedHelm)

	// 7. Execute the Pipeline
	targetPath := filepath.Join(casesDir, targetFileName)
	ctx := signal.SignalContextCreate(dispatcher)
	res := interpreter.HelmInterpreterInterpretFile(sharedHelm, targetPath, ctx)

	// Track the final error state across the requested pipeline depth
	var pipelineErr = res.Error

	// If interpretation succeeded AND we requested graph resolution, push the pipeline forward
	if res.Error == nil && targetToResolve != "" {
		_, pipelineErr = interpreter.HelmInterpreterDebugExecutionChainForTarget(res, ctx, targetToResolve)
	}

	// 8. Validate
	if collectErr != nil {
		t.Fatalf("failed to collect diagnostics for %s: %v", targetFileName, collectErr)
	}
	if pipelineErr == nil {
		t.Fatalf("expected %s to fail pipeline execution, but it succeeded", targetFileName)
	}
	if validate != nil {
		if err := validate(); err != nil {
			t.Fatalf("%s diagnostics mismatch: %v", targetFileName, err)
		}
	}

	// 9. Visual Output Execution
	fmt.Println("========================================")
	fmt.Printf(" MODE: NONE (%s)\n", targetFileName)
	fmt.Println("========================================")
	fmt.Print(rendering.SignalRendererRender(rendererNone))

	fmt.Println("========================================")
	fmt.Printf(" MODE: ANSI 16 (%s)\n", targetFileName)
	fmt.Println("========================================")
	fmt.Print(rendering.SignalRendererRender(rendererANSI))

	fmt.Println("========================================")
	fmt.Printf(" MODE: TRUE COLOR (%s)\n", targetFileName)
	fmt.Println("========================================")
	fmt.Print(rendering.SignalRendererRender(rendererTrue))
}
