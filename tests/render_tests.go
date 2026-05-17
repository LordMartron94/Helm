package tests

import (
	"encoding/json"
	"fmt"
	"foundation/location"
	"foundation/system"
	"helm/internal/ir"
	"helm/internal/targetexecutor"
	"helm/interpreter"
	"helm/shared"
	"os"
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

	executeVisualDiagnosticHarness(t, "bad_syntax.helm", squigglyRenderer, nil,
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

	executeVisualDiagnosticHarness(t, "bad_semantics.helm", squigglyRenderer, nil,
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

	executeVisualDiagnosticHarness(t, "bad_dag_cycle.helm", squigglyRenderer,
		func(res interpreter.HelmInterpreterInterpretationResult, ctx *signal.SignalContext) error {
			_, err := interpreter.HelmInterpreterDebugExecutionChainForTarget(res, ctx, "a")
			return err
		},
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

func HelmRenderTestCache(t *testing.T) {
	var cacheUpdatedTargets []string
	var skippedTargets []string

	casesDir, err := helmTestsCasesDirResolve()
	if err != nil {
		t.Fatalf("helm test setup failed (resolve cases dir): %v", err)
	}

	cacheRenderDir := filepath.Join(casesDir, "cache_render")
	fixtureDir := filepath.Join(cacheRenderDir, "fixtures")
	cacheStorePath := filepath.Join(cacheRenderDir, ".helm", "cache", "targets.json")

	// Only create missing fixture files — never overwrite manual edits.
	if err := cacheRenderSeedFixturesIfMissing(fixtureDir); err != nil {
		t.Fatalf("failed to seed cache_render fixtures: %v", err)
	}

	executeVisualDiagnosticHarness(t, "cache_render/cache.helm", shared.HelmExecutionOutputDetailHook(IntentMeta),
		func(res interpreter.HelmInterpreterInterpretationResult, ctx *signal.SignalContext) error {
			opts := targetexecutor.TargetExecutorOptions{}
			return interpreter.HelmInterpreterExecuteTarget(res, ctx, "render_downstream", nil, opts)
		},
		func(sig signal.Signal) error {
			phase, err := signal.SignalPayloadGetAs[string](&sig, shared.PhasePayloadKey)
			if err != nil || phase != shared.TargetExecutionPhase {
				return nil
			}

			target, _ := signal.SignalPayloadGetAs[string](&sig, shared.TargetPayloadKey)

			switch sig.ID() {
			case shared.SignalCacheUpdated:
				cacheUpdatedTargets = append(cacheUpdatedTargets, target)
			case shared.SignalExecSkipped:
				skippedTargets = append(skippedTargets, target)
			}

			return nil
		},
		func() error {
			content, err := os.ReadFile(cacheStorePath)
			if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("cache store at %s: %w", cacheStorePath, err)
			}

			recordCount := 0
			if err == nil {
				var records []map[string]any
				if jsonErr := json.Unmarshal(content, &records); jsonErr != nil {
					return fmt.Errorf("cache store is not valid JSON: %w", jsonErr)
				}
				recordCount = len(records)
			}

			fmt.Println()
			fmt.Println("--- Cache render demo (playground; state persists) ---")
			fmt.Printf("  Helm file:    %s\n", filepath.Join(cacheRenderDir, "cache.helm"))
			fmt.Printf("  Fixtures:     %s  (edit files here; test will not reset them)\n", fixtureDir)
			fmt.Printf("  Cache store:  %s\n", cacheStorePath)
			fmt.Printf("  Records:      %d target(s) on disk\n", recordCount)
			fmt.Printf("  This run:     CACHE_UPDATED=%v  EXEC_SKIPPED=%v\n", cacheUpdatedTargets, skippedTargets)
			fmt.Println()
			fmt.Println("  Manual play:")
			fmt.Println("    • Edit fixtures/input.txt  → invalidates render_upstream (and downstream via dep state)")
			fmt.Println("    • Edit fixtures/out.txt    → invalidates render_downstream (its input)")
			fmt.Println("    • Delete .helm/cache/      → cold cache, all targets run")
			fmt.Println("    • Re-run HelmRenderTestCache after each change (one execution per run)")
			fmt.Println()

			return nil
		},
	)
}

func cacheRenderSeedFixturesIfMissing(fixtureDir string) error {
	defaults := map[string]string{
		"input.txt":    "render-demo-input\n",
		"out.txt":      "render-demo-output\n",
		"down_out.txt": "render-demo-downstream\n",
	}
	for name, content := range defaults {
		path := filepath.Join(fixtureDir, name)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func HelmRenderTestExecution(t *testing.T) {
	var execOKStdout []string

	executeVisualDiagnosticHarness(t, "execution.helm", shared.HelmExecutionOutputDetailHook(IntentMeta),
		func(res interpreter.HelmInterpreterInterpretationResult, ctx *signal.SignalContext) error {
			opts := targetexecutor.TargetExecutorOptions{
				ConfirmDependency: func(dep string, target ir.HelmTargetDependency) (bool, error) {
					return true, nil
				},
			}

			return interpreter.HelmInterpreterExecuteTarget(res, ctx, "optional_recovery_node", nil, opts)
		},
		func(sig signal.Signal) error {
			if sig.ID() != shared.SignalExecOK {
				return nil
			}
			phase, err := signal.SignalPayloadGetAs[string](&sig, shared.PhasePayloadKey)
			if err != nil || phase != shared.TargetExecutionPhase {
				return nil
			}
			stdout, err := signal.SignalPayloadGetAs[string](&sig, shared.StdoutPayloadKey)
			if err != nil {
				return nil
			}
			execOKStdout = append(execOKStdout, stdout)
			return nil
		},
		func() error {
			for _, stdout := range execOKStdout {
				if strings.Contains(stdout, "cmd_recovery") {
					return nil
				}
			}
			return fmt.Errorf("expected EXEC_OK stdout containing cmd_recovery, got: %v", execOKStdout)
		},
	)
}

func executeVisualDiagnosticHarness(
	t *testing.T,
	targetFileName string,
	detailHook rendering.SignalDetailExtension,
	pipelineAction func(res interpreter.HelmInterpreterInterpretationResult, ctx *signal.SignalContext) error,
	onSignal func(sig signal.Signal) error,
	validate func() error,
) {
	// 1. Client builds the visual Palette
	paletteBuilder := splash.SPLASH_Rendering_TerminalPaletteBuilderCreate(int(intentCount))

	paletteBuilder.Register(IntentCategoryError, splash.SPLASH_Rendering_TerminalColorAnsi16_Red, splash.SPLASH_Rendering_TerminalTrueColor(231, 76, 60))
	paletteBuilder.Register(IntentCategoryWarning, splash.SPLASH_Rendering_TerminalColorAnsi16_Yellow, splash.SPLASH_Rendering_TerminalTrueColor(241, 196, 15))
	paletteBuilder.Register(IntentCategoryInfo, splash.SPLASH_Rendering_TerminalColorAnsi16_Cyan, splash.SPLASH_Rendering_TerminalTrueColor(52, 152, 219))
	paletteBuilder.Register(IntentDefault, splash.SPLASH_Rendering_TerminalColorAnsi16_BrightBlack, splash.SPLASH_Rendering_TerminalTrueColor(127, 140, 141))
	paletteBuilder.Register(IntentMeta, splash.SPLASH_Rendering_TerminalColorAnsi16_BrightBlack, splash.SPLASH_Rendering_TerminalTrueColor(127, 140, 141))

	palette := paletteBuilder.Build()

	// 2. Client initializes Splash engines
	splashNone := splash.SPLASH_Rendering_TerminalRendererCreate(splash.SPLASH_Rendering_TerminalColorModeNone, palette)
	splashANSI := splash.SPLASH_Rendering_TerminalRendererCreate(splash.SPLASH_Rendering_TerminalColorModeAnsi16, palette)
	splashTrue := splash.SPLASH_Rendering_TerminalRendererCreate(splash.SPLASH_Rendering_TerminalColorModeTrueColor, palette)

	// 3. Client Location Formatter
	clientLocationFormatter := func(loc *location.Location) string {
		startLine, errSL := location.LocationCoordinateGetAs[int](*loc, "start_line")
		startCol, errSC := location.LocationCoordinateGetAs[int](*loc, "start_column")
		if errSL == nil && errSC == nil && startLine > 0 {
			return fmt.Sprintf(" at %d:%d", startLine, startCol)
		}
		return ""
	}

	// 4. Client initializes the Adapters
	rendererNone := rendering.SignalRendererCreate(splashNone, fileGroupingStrategy, clientLocationFormatter, detailHook, IntentMeta)
	rendererANSI := rendering.SignalRendererCreate(splashANSI, fileGroupingStrategy, clientLocationFormatter, detailHook, IntentMeta)
	rendererTrue := rendering.SignalRendererCreate(splashTrue, fileGroupingStrategy, clientLocationFormatter, detailHook, IntentMeta)

	// 5. Setup the Dispatcher
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

	// 6. Bootstrap the Interpreter
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

	var pipelineErr error
	if res.Error == nil && pipelineAction != nil {
		pipelineErr = pipelineAction(res, ctx)
	}

	if collectErr != nil {
		t.Fatalf("failed to collect diagnostics for %s: %v", targetFileName, collectErr)
	}

	if pipelineErr == nil && validate != nil {
		if err := validate(); err != nil {
			t.Fatalf("%s diagnostics mismatch: %v", targetFileName, err)
		}
	}

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

	if pipelineErr != nil {
		t.Fatalf("%s pipeline failed: %v", targetFileName, pipelineErr)
	}
}
