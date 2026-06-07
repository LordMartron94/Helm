package tests

import (
	"fmt"
	"foundation/location"
	"helm/internal/ir"
	"helm/internal/targetexecutor"
	"helm/interpreter"
	"helm/shared"
	"lingua/helm"
	"os"
	"path/filepath"
	"shield"
	"signal"
	"strings"
	"sync"
)

var standardRunCfg = shield.SHIELD_Testing_ScenarioRunConfig{MaxIterations: 1}

// Target execution emits INFO signals (EXEC_OK, EXEC_SKIPPED, CACHE_UPDATED).
var helmExecutionSignalManifest = signal.DiagnosticCategoryManifest{
	{Label: "INFO", Weight: 0},
	{Label: "ERROR", Weight: 20},
}

func init() {
	op := shield.SHIELD_Testing_OperationCreateStateless(
		"operation_helm_interpreter",
		"Validates the Helm interpreter syntax, lifecycle, and execution",
		runHelmOperation,
		"HELM", "Interpreter",
	)

	shield.SHIELD_Registry_OperationRegister(op)
}

func runHelmOperation(_ struct{}, execCtx shield.SHIELD_Testing_ExecutionContext) []shield.SHIELD_Testing_ScenarioRunResult {
	var results []shield.SHIELD_Testing_ScenarioRunResult

	specPath, releaseSpec, specErr := helmLSpecPathResolve()
	casesDir, casesErr := helmTestsCasesDirResolve()

	if specErr != nil {
		panic(fmt.Sprintf("helm test setup failed (resolve spec): %v", specErr))
	}
	if releaseSpec != nil {
		defer releaseSpec()
	}
	if casesErr != nil {
		panic(fmt.Sprintf("helm test setup failed (resolve cases dir): %v", casesErr))
	}

	sharedHelm, createErr := interpreter.HelmInterpreterTryCreate(specPath)

	results = append(results, runCreationScenario(execCtx, sharedHelm, createErr))

	if sharedHelm == nil {
		return results
	}

	defer interpreter.HelmInterpreterDestroy(sharedHelm)

	results = append(results, runSyntaxScenario(execCtx, sharedHelm, casesDir))
	results = append(results, runInvalidSyntaxScenario(execCtx, sharedHelm, casesDir))
	results = append(results, runInvalidSemanticsScenario(execCtx, sharedHelm, casesDir))
	results = append(results, runValidPathsScenario(execCtx, sharedHelm, casesDir))
	results = append(results, runValidGlobsScenario(execCtx, sharedHelm, casesDir))
	results = append(results, runDAGResolutionScenario(execCtx, sharedHelm, casesDir))
	results = append(results, runTargetExecutionScenario(execCtx, sharedHelm, casesDir))
	results = append(results, runCacheScenario(execCtx, sharedHelm, casesDir))

	return results
}

// ------------------------------------------------------------------ SCENARIOS

type creationScenarioInput struct{}
type creationScenarioOutput struct {
	err   error
	isNil bool
}

func runCreationScenario(
	execCtx shield.SHIELD_Testing_ExecutionContext,
	sharedHelm *interpreter.HelmInterpreter,
	createErr error,
) shield.SHIELD_Testing_ScenarioRunResult {

	scenario := shield.SHIELD_Testing_ScenarioCreate(
		"scenario_helm_creation",
		"Validates the Helm interpreter is created successfully and is not nil",
		[]shield.SHIELD_Testing_Guard[creationScenarioInput, creationScenarioOutput]{
			shield.SHIELD_Testing_GuardCreate(
				"guard_interpreter_ready",
				creationScenarioInput{},
				shield.SHIELD_Testing_GuardPolicyPredicate(func(out creationScenarioOutput) (bool, string) {
					if out.err != nil {
						return false, fmt.Sprintf("creation returned error: %v", out.err)
					}
					if out.isNil {
						return false, "interpreter instance is nil"
					}
					return true, ""
				}),
			),
		},
		func(_ creationScenarioInput) (creationScenarioOutput, error) {
			return creationScenarioOutput{
				err:   createErr,
				isNil: sharedHelm == nil,
			}, nil
		},
	)

	return shield.SHIELD_Testing_OperationRunScenario(scenario, execCtx, standardRunCfg)
}

type syntaxScenarioInput struct {
	fileName string
}
type syntaxScenarioOutput struct {
	errorMsg string
}

func runSyntaxScenario(
	execCtx shield.SHIELD_Testing_ExecutionContext,
	sharedHelm *interpreter.HelmInterpreter,
	casesDir string,
) shield.SHIELD_Testing_ScenarioRunResult {

	scenario := shield.SHIELD_Testing_ScenarioCreate(
		"scenario_helm_syntax",
		"Validates all intended syntax does not error during the parsing phase",
		buildSyntaxGuards(),
		func(input syntaxScenarioInput) (syntaxScenarioOutput, error) {
			path := filepath.Join(casesDir, input.fileName)

			dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
				{Label: "ERROR", Weight: 20},
			})
			ctx := signal.SignalContextCreate(dispatcher)

			res := interpreter.HelmInterpreterInterpretFile(sharedHelm, path, ctx)

			if res.Error != nil {
				return syntaxScenarioOutput{errorMsg: res.Error.Error()}, nil
			}

			return syntaxScenarioOutput{errorMsg: ""}, nil
		},
	)

	return shield.SHIELD_Testing_OperationRunScenario(scenario, execCtx, standardRunCfg)
}

func buildSyntaxGuards() []shield.SHIELD_Testing_Guard[syntaxScenarioInput, syntaxScenarioOutput] {
	type testCase struct {
		name string
		file string
	}

	cases := []testCase{
		{"guard_syntax_empty_program", "empty_program.helm"},
		{"guard_syntax_newline", "newline.helm"},
		{"guard_syntax_comments", "comments.helm"},
		{"guard_syntax_variables", "variables.helm"},
		{"guard_syntax_multiline_string", "multiline_string.helm"},
		{"guard_syntax_empty_string", "empty_string.helm"},
		{"guard_syntax_targets", "targets.helm"},
	}

	var guards []shield.SHIELD_Testing_Guard[syntaxScenarioInput, syntaxScenarioOutput]

	for _, c := range cases {
		guards = append(guards, shield.SHIELD_Testing_GuardCreate(
			c.name,
			syntaxScenarioInput{fileName: c.file},
			shield.SHIELD_Testing_GuardPolicyPredicate(func(out syntaxScenarioOutput) (bool, string) {
				if out.errorMsg != "" && !strings.Contains(out.errorMsg, "semantic") {
					return false, out.errorMsg
				}
				return true, ""
			}),
		))
	}

	return guards
}

type invalidSyntaxScenarioInput struct {
	fileName string
}
type invalidSyntaxScenarioOutput struct {
	errorMsg     string
	diagnostics  []helmSyntaxDiagnostic
	collectError string
}

type helmSyntaxDiagnostic struct {
	startLine int
	rule      string
	message   string
}

type helmSyntaxErrorExpectation struct {
	line            int
	ruleContains    string
	messageContains string
}

var badSyntaxErrorExpectations = []helmSyntaxErrorExpectation{
	{line: 3, ruleContains: "DEPENDS_ON", messageContains: "expected TokBracketOpen"},
	{line: 7, ruleContains: "CONDITIONAL_BLOCK", messageContains: "unexpected TokKWWhen, expected 'TokBraceClose'"},
	{line: 13, ruleContains: "RUN", messageContains: "expected STRING_LITERAL"},
}

func runInvalidSyntaxScenario(
	execCtx shield.SHIELD_Testing_ExecutionContext,
	sharedHelm *interpreter.HelmInterpreter,
	casesDir string,
) shield.SHIELD_Testing_ScenarioRunResult {

	scenario := shield.SHIELD_Testing_ScenarioCreate(
		"scenario_helm_invalid_syntax",
		"Validates that malformed helm files correctly trigger parsing or lexical errors",
		[]shield.SHIELD_Testing_Guard[invalidSyntaxScenarioInput, invalidSyntaxScenarioOutput]{
			shield.SHIELD_Testing_GuardCreate(
				"guard_syntax_must_fail",
				invalidSyntaxScenarioInput{fileName: "bad_syntax.helm"},
				shield.SHIELD_Testing_GuardPolicyPredicate(func(out invalidSyntaxScenarioOutput) (bool, string) {
					if out.collectError != "" {
						return false, out.collectError
					}
					if out.errorMsg == "" {
						return false, "expected a syntax error from malformed file, but interpretation succeeded"
					}
					return helmBadSyntaxDiagnosticsMatch(out.diagnostics)
				}),
			),
		},
		func(input invalidSyntaxScenarioInput) (invalidSyntaxScenarioOutput, error) {
			path := filepath.Join(casesDir, input.fileName)

			dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
				{Label: "ERROR", Weight: 20},
			})
			var collected []helmSyntaxDiagnostic
			var collectErr string
			signal.SignalDispatcherRegisterSink(dispatcher, "collect", func(sig signal.Signal) {
				diag, err := helmSyntaxDiagnosticFromSignal(sig)
				if err != nil {
					if collectErr == "" {
						collectErr = err.Error()
					}
					return
				}
				collected = append(collected, diag)
			})
			ctx := signal.SignalContextCreate(dispatcher)

			res := interpreter.HelmInterpreterInterpretFile(sharedHelm, path, ctx)

			out := invalidSyntaxScenarioOutput{
				diagnostics:  collected,
				collectError: collectErr,
			}
			if res.Error != nil {
				out.errorMsg = res.Error.Error()
			}
			return out, nil
		},
	)

	return shield.SHIELD_Testing_OperationRunScenario(scenario, execCtx, standardRunCfg)
}

type invalidSemanticsScenarioInput struct {
	fileName string
}
type invalidSemanticsScenarioOutput struct {
	errorMsg     string
	diagnostics  []helmSemanticDiagnostic
	collectError string
}

type helmSemanticDiagnostic struct {
	startLine int
	message   string
}

type helmSemanticErrorExpectation struct {
	messageContains string
}

var badSemanticsExpectations = []helmSemanticErrorExpectation{
	{messageContains: "variable 'VERSION' has already been declared"},
	{messageContains: "target 'build' has already been declared"},
	{messageContains: "missing_target"},
	{messageContains: "use of undeclared variable 'UNDEFINED_VAR'"},
	{messageContains: "condition references undeclared name 'UNKNOWN_PARAM' (not a global or target parameter)"},
	{messageContains: "unknown keyword argument 'unknown'"},
}

func runInvalidSemanticsScenario(
	execCtx shield.SHIELD_Testing_ExecutionContext,
	sharedHelm *interpreter.HelmInterpreter,
	casesDir string,
) shield.SHIELD_Testing_ScenarioRunResult {

	scenario := shield.SHIELD_Testing_ScenarioCreate(
		"scenario_helm_invalid_semantics",
		"Validates that syntactically valid files with semantic domain errors trigger precise diagnostic signals",
		[]shield.SHIELD_Testing_Guard[invalidSemanticsScenarioInput, invalidSemanticsScenarioOutput]{
			shield.SHIELD_Testing_GuardCreate(
				"guard_semantics_must_fail",
				invalidSemanticsScenarioInput{fileName: "bad_semantics.helm"},
				shield.SHIELD_Testing_GuardPolicyPredicate(func(out invalidSemanticsScenarioOutput) (bool, string) {
					if out.collectError != "" {
						return false, out.collectError
					}
					if out.errorMsg == "" {
						return false, "expected interpreter to return an error due to semantic violations, but it succeeded"
					}
					return helmBadSemanticsDiagnosticsMatch(out.diagnostics)
				}),
			),
		},
		func(input invalidSemanticsScenarioInput) (invalidSemanticsScenarioOutput, error) {
			path := filepath.Join(casesDir, input.fileName)

			dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
				{Label: "ERROR", Weight: 20},
			})

			var collected []helmSemanticDiagnostic
			var collectErr string

			signal.SignalDispatcherRegisterSink(dispatcher, "collect", func(sig signal.Signal) {
				// We only care about semantic errors, ignore syntax or lexical signals if any bleed through
				phase, _ := signal.SignalPayloadGetAs[string](&sig, shared.PhasePayloadKey)
				if phase != shared.SemanticAnalysisPhase {
					return
				}

				diag, err := helmSemanticDiagnosticFromSignal(sig)
				if err != nil {
					if collectErr == "" {
						collectErr = err.Error()
					}
					return
				}
				collected = append(collected, diag)
			})

			ctx := signal.SignalContextCreate(dispatcher)

			res := interpreter.HelmInterpreterInterpretFile(sharedHelm, path, ctx)

			out := invalidSemanticsScenarioOutput{
				diagnostics:  collected,
				collectError: collectErr,
			}
			if res.Error != nil {
				out.errorMsg = res.Error.Error()
			}
			return out, nil
		},
	)

	return shield.SHIELD_Testing_OperationRunScenario(scenario, execCtx, standardRunCfg)
}

func helmSemanticDiagnosticFromSignal(sig signal.Signal) (helmSemanticDiagnostic, error) {
	message, err := signal.SignalPayloadGetAs[string](&sig, "message")
	if err != nil {
		return helmSemanticDiagnostic{}, fmt.Errorf("semantic signal missing message payload: %w", err)
	}

	if !sig.HasLocation() {
		return helmSemanticDiagnostic{}, fmt.Errorf("semantic signal missing location")
	}
	loc := sig.Location()
	if loc == nil {
		return helmSemanticDiagnostic{}, fmt.Errorf("semantic signal location is nil")
	}

	startLine, err := location.LocationCoordinateGetAs[int](*loc, "start_line")
	if err != nil {
		return helmSemanticDiagnostic{}, fmt.Errorf("semantic signal missing start_line: %w", err)
	}

	return helmSemanticDiagnostic{
		startLine: startLine,
		message:   message,
	}, nil
}

func helmBadSemanticsDiagnosticsMatch(got []helmSemanticDiagnostic) (bool, string) {
	want := badSemanticsExpectations

	for _, exp := range want {
		if !helmSemanticDiagnosticMatchesExpectation(got, exp) {
			return false, fmt.Sprintf(
				"missing expected semantic error (message contains %q);\ngot:\n%s",
				exp.messageContains, helmSemanticDiagnosticsFormat(got),
			)
		}
	}

	for _, diag := range got {
		isExpected := false
		for _, exp := range want {
			if strings.Contains(diag.message, exp.messageContains) {
				isExpected = true
				break
			}
		}

		if !isExpected {
			return false, fmt.Sprintf(
				"unexpected semantic error emitted:\n  L%d msg=%q\n\nall got:\n%s",
				diag.startLine, diag.message, helmSemanticDiagnosticsFormat(got),
			)
		}
	}

	return true, ""
}

func helmSemanticDiagnosticMatchesExpectation(got []helmSemanticDiagnostic, exp helmSemanticErrorExpectation) bool {
	for _, diag := range got {
		if strings.Contains(diag.message, exp.messageContains) {
			return true
		}
	}
	return false
}

func helmSemanticDiagnosticsFormat(diags []helmSemanticDiagnostic) string {
	if len(diags) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(diags))
	for _, d := range diags {
		parts = append(parts, fmt.Sprintf("L%d msg=%q", d.startLine, d.message))
	}
	return strings.Join(parts, "\n")
}

func helmSyntaxDiagnosticFromSignal(sig signal.Signal) (helmSyntaxDiagnostic, error) {
	message, err := signal.SignalPayloadGetAs[string](&sig, "message")
	if err != nil {
		return helmSyntaxDiagnostic{}, fmt.Errorf("syntax signal missing message payload: %w", err)
	}
	rule, err := signal.SignalPayloadGetAs[string](&sig, "rule")
	if err != nil {
		return helmSyntaxDiagnostic{}, fmt.Errorf("syntax signal missing rule payload: %w", err)
	}
	if !sig.HasLocation() {
		return helmSyntaxDiagnostic{}, fmt.Errorf("syntax signal missing location")
	}
	loc := sig.Location()
	if loc == nil {
		return helmSyntaxDiagnostic{}, fmt.Errorf("syntax signal location is nil")
	}
	startLine, err := location.LocationCoordinateGetAs[int](*loc, "start_line")
	if err != nil {
		return helmSyntaxDiagnostic{}, fmt.Errorf("syntax signal missing start_line: %w", err)
	}
	return helmSyntaxDiagnostic{
		startLine: startLine,
		rule:      rule,
		message:   message,
	}, nil
}

func helmBadSyntaxDiagnosticsMatch(got []helmSyntaxDiagnostic) (bool, string) {
	want := badSyntaxErrorExpectations
	if len(got) != len(want) {
		return false, fmt.Sprintf(
			"expected %d syntax diagnostics, got %d: %s",
			len(want), len(got), helmSyntaxDiagnosticsFormat(got),
		)
	}

	for _, exp := range want {
		if !helmSyntaxDiagnosticMatchesExpectation(got, exp) {
			return false, fmt.Sprintf(
				"missing expected syntax error at line %d (rule contains %q, message contains %q); got: %s",
				exp.line, exp.ruleContains, exp.messageContains, helmSyntaxDiagnosticsFormat(got),
			)
		}
	}

	for _, diag := range got {
		if !helmSyntaxDiagnosticLineIsExpected(diag.startLine) {
			return false, fmt.Sprintf(
				"unexpected syntax error at line %d (rule=%q message=%q)",
				diag.startLine, diag.rule, diag.message,
			)
		}
	}

	return true, ""
}

func helmSyntaxDiagnosticMatchesExpectation(got []helmSyntaxDiagnostic, exp helmSyntaxErrorExpectation) bool {
	for _, diag := range got {
		if diag.startLine != exp.line {
			continue
		}
		if !strings.Contains(diag.rule, exp.ruleContains) {
			continue
		}
		if !strings.Contains(diag.message, exp.messageContains) {
			continue
		}
		return true
	}
	return false
}

func helmSyntaxDiagnosticLineIsExpected(line int) bool {
	for _, exp := range badSyntaxErrorExpectations {
		if exp.line == line {
			return true
		}
	}
	return false
}

func helmSyntaxDiagnosticsFormat(diags []helmSyntaxDiagnostic) string {
	if len(diags) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(diags))
	for _, d := range diags {
		parts = append(parts, fmt.Sprintf("L%d rule=%q msg=%q", d.startLine, d.rule, d.message))
	}
	return strings.Join(parts, "; ")
}

type validPathsScenarioInput struct {
	fileName string
}
type validPathsScenarioOutput struct {
	extractedOutputs []ir.HelmArtifactInput
	parseError       error
}

func runValidPathsScenario(
	execCtx shield.SHIELD_Testing_ExecutionContext,
	sharedHelm *interpreter.HelmInterpreter,
	casesDir string,
) shield.SHIELD_Testing_ScenarioRunResult {

	scenario := shield.SHIELD_Testing_ScenarioCreate(
		"scenario_helm_valid_paths",
		"Validates that the IR Builder correctly evaluates path() arguments into native OS paths",
		[]shield.SHIELD_Testing_Guard[validPathsScenarioInput, validPathsScenarioOutput]{
			shield.SHIELD_Testing_GuardCreate(
				"guard_paths_must_resolve",
				validPathsScenarioInput{fileName: "valid_paths.helm"},
				shield.SHIELD_Testing_GuardPolicyPredicate(func(out validPathsScenarioOutput) (bool, string) {
					if out.parseError != nil {
						return false, fmt.Sprintf("expected successful parse, got error: %v", out.parseError)
					}

					if len(out.extractedOutputs) != 2 {
						return false, fmt.Sprintf("expected 2 outputs, got %d: %v", len(out.extractedOutputs), out.extractedOutputs)
					}

					// Mathematically define what the paths SHOULD be using the native OS separator
					expectedPath1 := filepath.Join("build_output", "bin", "executable")
					expectedPath2 := filepath.Join("static", "index.html")

					if out.extractedOutputs[0].Literal != expectedPath1 {
						return false, fmt.Sprintf("path 1 mismatch: expected %q, got %q", expectedPath1, out.extractedOutputs[0].Literal)
					}

					if out.extractedOutputs[1].Literal != expectedPath2 {
						return false, fmt.Sprintf("path 2 mismatch: expected %q, got %q", expectedPath2, out.extractedOutputs[1].Literal)
					}

					return true, ""
				}),
			),
		},
		func(input validPathsScenarioInput) (validPathsScenarioOutput, error) {
			path := filepath.Join(casesDir, input.fileName)

			// Create a black-hole dispatcher. We expect no semantic errors here.
			dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
				{Label: "ERROR", Weight: 20},
			})
			ctx := signal.SignalContextCreate(dispatcher)

			res := interpreter.HelmInterpreterInterpretFile(sharedHelm, path, ctx)

			if res.Error != nil {
				return validPathsScenarioOutput{parseError: res.Error}, nil
			}

			// Extract the compiled IR data
			target, exists := res.BuiltIR.Targets["compile"]
			if !exists {
				return validPathsScenarioOutput{parseError: fmt.Errorf("target 'compile' not found in IR")}, nil
			}

			if target.Artifacts == nil {
				return validPathsScenarioOutput{parseError: fmt.Errorf("artifacts block was nil")}, nil
			}

			return validPathsScenarioOutput{
				extractedOutputs: target.Artifacts.Outputs,
			}, nil
		},
	)

	return shield.SHIELD_Testing_OperationRunScenario(scenario, execCtx, standardRunCfg)
}

type validGlobsScenarioInput struct {
	fileName string
}
type validGlobsScenarioOutput struct {
	extractedInputs []ir.HelmArtifactInput
	parseError      error
}

func runValidGlobsScenario(
	execCtx shield.SHIELD_Testing_ExecutionContext,
	sharedHelm *interpreter.HelmInterpreter,
	casesDir string,
) shield.SHIELD_Testing_ScenarioRunResult {

	scenario := shield.SHIELD_Testing_ScenarioCreate(
		"scenario_helm_valid_globs",
		"Validates that the IR Builder correctly evaluates glob() into structured HelmGlob values",
		[]shield.SHIELD_Testing_Guard[validGlobsScenarioInput, validGlobsScenarioOutput]{
			shield.SHIELD_Testing_GuardCreate(
				"guard_globs_must_resolve",
				validGlobsScenarioInput{fileName: "valid_globs.helm"},
				shield.SHIELD_Testing_GuardPolicyPredicate(func(out validGlobsScenarioOutput) (bool, string) {
					if out.parseError != nil {
						return false, fmt.Sprintf("expected successful parse, got error: %v", out.parseError)
					}

					if len(out.extractedInputs) != 3 {
						return false, fmt.Sprintf("expected 3 inputs, got %d", len(out.extractedInputs))
					}

					if out.extractedInputs[0].Kind != ir.ArtifactInputGlob {
						return false, "input 0: expected glob entry"
					}
					glob0 := out.extractedInputs[0].Glob
					if glob0 == nil {
						return false, "input 0: glob is nil"
					}
					if glob0.BaseDirectory != "libs" {
						return false, fmt.Sprintf("input 0 base dir: expected %q, got %q", "libs", glob0.BaseDirectory)
					}
					if len(glob0.Includes) != 1 || glob0.Includes[0] != "**/*.go" {
						return false, fmt.Sprintf("input 0 includes: expected [%q], got %v", "**/*.go", glob0.Includes)
					}
					if len(glob0.Excludes) != 0 {
						return false, fmt.Sprintf("input 0 excludes: expected empty, got %v", glob0.Excludes)
					}
					if glob0.FollowSymlinks {
						return false, "input 0 follow_symlinks: expected default false"
					}
					if !glob0.Recursive {
						return false, "input 0 recursive: expected default true"
					}
					if glob0.Types != "files" {
						return false, fmt.Sprintf("input 0 types: expected default %q, got %q", "files", glob0.Types)
					}

					if out.extractedInputs[1].Kind != ir.ArtifactInputGlob {
						return false, "input 1: expected glob entry"
					}
					glob1 := out.extractedInputs[1].Glob
					if glob1 == nil {
						return false, "input 1: glob is nil"
					}
					if glob1.BaseDirectory != "." {
						return false, fmt.Sprintf("input 1 base dir: expected %q, got %q", ".", glob1.BaseDirectory)
					}
					if len(glob1.Excludes) != 1 || glob1.Excludes[0] != "*_test.go" {
						return false, fmt.Sprintf("input 1 excludes: expected [%q], got %v", "*_test.go", glob1.Excludes)
					}
					if len(glob1.Includes) != 0 {
						return false, fmt.Sprintf("input 1 includes: expected empty, got %v", glob1.Includes)
					}
					if glob1.FollowSymlinks || !glob1.Recursive || glob1.Types != "files" {
						return false, "input 1: expected default follow_symlinks=false, recursive=true, types=files"
					}

					glob2 := out.extractedInputs[2].Glob
					if glob2 == nil {
						return false, "input 2: glob is nil"
					}
					if glob2.BaseDirectory != "vendor" {
						return false, fmt.Sprintf("input 2 base dir: expected %q, got %q", "vendor", glob2.BaseDirectory)
					}
					if !glob2.FollowSymlinks {
						return false, "input 2 follow_symlinks: expected true"
					}
					if glob2.Recursive {
						return false, "input 2 recursive: expected false"
					}
					if glob2.Types != "directories" {
						return false, fmt.Sprintf("input 2 types: expected %q, got %q", "directories", glob2.Types)
					}

					return true, ""
				}),
			),
		},
		func(input validGlobsScenarioInput) (validGlobsScenarioOutput, error) {
			path := filepath.Join(casesDir, input.fileName)

			dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
				{Label: "ERROR", Weight: 20},
			})
			ctx := signal.SignalContextCreate(dispatcher)

			res := interpreter.HelmInterpreterInterpretFile(sharedHelm, path, ctx)

			if res.Error != nil {
				return validGlobsScenarioOutput{parseError: res.Error}, nil
			}

			target, exists := res.BuiltIR.Targets["scan"]
			if !exists {
				return validGlobsScenarioOutput{parseError: fmt.Errorf("target 'scan' not found in IR")}, nil
			}

			if target.Artifacts == nil {
				return validGlobsScenarioOutput{parseError: fmt.Errorf("artifacts block was nil")}, nil
			}

			return validGlobsScenarioOutput{
				extractedInputs: target.Artifacts.Inputs,
			}, nil
		},
	)

	return shield.SHIELD_Testing_OperationRunScenario(scenario, execCtx, standardRunCfg)
}

type dagScenarioInput struct {
	fileName   string
	targetName string
}

type dagScenarioOutput struct {
	chain       [][]string
	err         error
	diagnostics []string // Capture the emitted error messages
}

func runDAGResolutionScenario(
	execCtx shield.SHIELD_Testing_ExecutionContext,
	sharedHelm *interpreter.HelmInterpreter,
	casesDir string,
) shield.SHIELD_Testing_ScenarioRunResult {

	scenario := shield.SHIELD_Testing_ScenarioCreate(
		"scenario_helm_dag_resolution",
		"Validates that the target execution chain resolves dependencies or catches topological errors",
		[]shield.SHIELD_Testing_Guard[dagScenarioInput, dagScenarioOutput]{
			shield.SHIELD_Testing_GuardCreate(
				"guard_valid_execution_chain",
				dagScenarioInput{fileName: "valid_dag.helm", targetName: "build"},
				shield.SHIELD_Testing_GuardPolicyPredicate(func(out dagScenarioOutput) (bool, string) {
					if out.err != nil {
						return false, fmt.Sprintf("expected successful DAG resolution, got error: %v", out.err)
					}

					layerMap := make(map[string]int)
					for layerIndex, parallelGroup := range out.chain {
						for _, target := range parallelGroup {
							layerMap[target] = layerIndex
						}
					}

					requiredTargets := []string{"db_up", "cache_up", "migrate", "build"}
					for _, req := range requiredTargets {
						if _, exists := layerMap[req]; !exists {
							return false, fmt.Sprintf("target '%s' is missing from the execution chain", req)
						}
					}

					if layerMap["db_up"] >= layerMap["migrate"] {
						return false, "topological violation: 'db_up' must execute before 'migrate'"
					}
					if layerMap["migrate"] >= layerMap["build"] {
						return false, "topological violation: 'migrate' must execute before 'build'"
					}
					if layerMap["cache_up"] >= layerMap["build"] {
						return false, "topological violation: 'cache_up' must execute before 'build'"
					}

					return true, ""
				}),
			),
			shield.SHIELD_Testing_GuardCreate(
				"guard_cyclic_dependency_fails",
				dagScenarioInput{fileName: "bad_dag_cycle.helm", targetName: "a"},
				shield.SHIELD_Testing_GuardPolicyPredicate(func(out dagScenarioOutput) (bool, string) {
					if out.err == nil {
						return false, "expected DAG resolution to fail due to a cycle, but it succeeded"
					}

					if len(out.diagnostics) == 0 {
						return false, "expected a graph resolution signal to be emitted, but none were collected"
					}

					foundCycleSignal := false
					for _, msg := range out.diagnostics {
						if strings.Contains(strings.ToLower(msg), "cycle") {
							foundCycleSignal = true
							break
						}
					}

					if !foundCycleSignal {
						return false, fmt.Sprintf("expected a cycle error diagnostic, got: %v", out.diagnostics)
					}

					return true, ""
				}),
			),
		},
		func(input dagScenarioInput) (dagScenarioOutput, error) {
			path := filepath.Join(casesDir, input.fileName)

			dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
				{Label: "ERROR", Weight: 20},
			})

			var collected []string
			signal.SignalDispatcherRegisterSink(dispatcher, "collect_graph_errors", func(sig signal.Signal) {
				phase, err := signal.SignalPayloadGetAs[string](&sig, shared.PhasePayloadKey)
				if err != nil || phase != shared.GraphResolutionPhase {
					return
				}

				msg, err := signal.SignalPayloadGetAs[string](&sig, shared.MessagePayloadKey)
				if err == nil {
					collected = append(collected, msg)
				}
			})

			ctx := signal.SignalContextCreate(dispatcher)

			res := interpreter.HelmInterpreterInterpretFile(sharedHelm, path, ctx)
			if res.Error != nil {
				return dagScenarioOutput{err: res.Error}, nil
			}

			// Pass the newly required ctx parameter
			chain, err := interpreter.HelmInterpreterDebugExecutionChainForTarget(res, ctx, input.targetName)

			return dagScenarioOutput{
				chain:       chain,
				err:         err,
				diagnostics: collected,
			}, nil
		},
	)

	return shield.SHIELD_Testing_OperationRunScenario(scenario, execCtx, standardRunCfg)
}

type executionScenarioInput struct {
	entryTarget     string
	parameters      map[string]string
	confirmResponse bool
}

type executionScenarioOutput struct {
	executedCommands []string
	pipelineErr      error
}

func runTargetExecutionScenario(
	execCtx shield.SHIELD_Testing_ExecutionContext,
	sharedHelm *interpreter.HelmInterpreter,
	casesDir string,
) shield.SHIELD_Testing_ScenarioRunResult {

	scenario := shield.SHIELD_Testing_ScenarioCreate(
		"scenario_helm_target_execution",
		"Validates graph execution, variable interpolation, optional dependencies, and conditionals",
		[]shield.SHIELD_Testing_Guard[executionScenarioInput, executionScenarioOutput]{
			shield.SHIELD_Testing_GuardCreate(
				"guard_exec_interpolation",
				executionScenarioInput{entryTarget: "params_target", parameters: map[string]string{"USER": "admin"}, confirmResponse: true},
				shield.SHIELD_Testing_GuardPolicyPredicate(func(out executionScenarioOutput) (bool, string) {
					if out.pipelineErr != nil {
						return false, fmt.Sprintf("expected success, got: %v", out.pipelineErr)
					}
					if !containsString(out.executedCommands, "cmd_base") {
						return false, "base target did not execute"
					}
					if !containsString(out.executedCommands, "cmd_params admin production") {
						return false, fmt.Sprintf("interpolation failed or target did not run. Got: %v", out.executedCommands)
					}
					return true, ""
				}),
			),
			shield.SHIELD_Testing_GuardCreate(
				"guard_exec_blocked_dependency",
				executionScenarioInput{entryTarget: "blocked_node", parameters: nil, confirmResponse: true},
				shield.SHIELD_Testing_GuardPolicyPredicate(func(out executionScenarioOutput) (bool, string) {
					if out.pipelineErr == nil {
						return false, "expected pipeline to fail due to failing dependency, but it succeeded"
					}
					if containsString(out.executedCommands, "cmd_blocked") {
						return false, "blocked target executed despite failed dependency"
					}
					return true, ""
				}),
			),
			shield.SHIELD_Testing_GuardCreate(
				"guard_exec_optional_recovery",
				executionScenarioInput{entryTarget: "optional_recovery_node", parameters: nil, confirmResponse: true},
				shield.SHIELD_Testing_GuardPolicyPredicate(func(out executionScenarioOutput) (bool, string) {
					if out.pipelineErr != nil {
						return false, fmt.Sprintf("expected success due to optional fallback, got: %v", out.pipelineErr)
					}
					if !containsString(out.executedCommands, "cmd_recovery") {
						return false, "optional fallback target did not execute"
					}
					return true, ""
				}),
			),
			shield.SHIELD_Testing_GuardCreate(
				"guard_exec_conditional_gating",
				executionScenarioInput{entryTarget: "conditions_target", parameters: map[string]string{"MODE": "active"}, confirmResponse: true},
				shield.SHIELD_Testing_GuardPolicyPredicate(func(out executionScenarioOutput) (bool, string) {
					if out.pipelineErr != nil {
						return false, fmt.Sprintf("expected success, got: %v", out.pipelineErr)
					}
					if !containsString(out.executedCommands, "cmd_mode_active active") {
						return false, "when defined() block failed to execute"
					}
					if containsString(out.executedCommands, "cmd_should_never_run") {
						return false, "when equals() block executed incorrectly"
					}
					return true, ""
				}),
			),
			shield.SHIELD_Testing_GuardCreate(
				"guard_exec_confirmation_denied",
				executionScenarioInput{entryTarget: "confirm_node", parameters: nil, confirmResponse: false}, // USER DENIES
				shield.SHIELD_Testing_GuardPolicyPredicate(func(out executionScenarioOutput) (bool, string) {
					if out.pipelineErr == nil {
						return false, "expected pipeline to abort due to denied confirmation"
					}
					if !strings.Contains(out.pipelineErr.Error(), "user declined") {
						return false, fmt.Sprintf("expected user decline error, got: %v", out.pipelineErr)
					}
					if containsString(out.executedCommands, "cmd_confirmed") {
						return false, "target executed despite user denying the dependency"
					}
					return true, ""
				}),
			),
		},
		func(input executionScenarioInput) (executionScenarioOutput, error) {
			path := filepath.Join(casesDir, "execution.helm")

			dispatcher := signal.SignalDispatcherCreate(helmExecutionSignalManifest)
			ctx := signal.SignalContextCreate(dispatcher)

			res := interpreter.HelmInterpreterInterpretFile(sharedHelm, path, ctx)
			if res.Error != nil {
				return executionScenarioOutput{pipelineErr: res.Error}, nil
			}

			// 1. Thread-safe execution tracker
			var mu sync.Mutex
			var executed []string

			// 2. The Mock Runner
			mockHandler := func(req targetexecutor.TargetRunRequest) (targetexecutor.TargetRunResult, error) {
				mu.Lock()
				executed = append(executed, req.Command)
				mu.Unlock()

				if strings.Contains(req.Command, "cmd_fail") {
					return targetexecutor.TargetRunResult{}, fmt.Errorf("mock simulated process failure")
				}
				return targetexecutor.TargetRunResult{}, nil
			}

			// 3. The Mock Confirmation Prompt
			mockConfirm := func(dependent string, dep ir.HelmTargetDependency) (bool, error) {
				return input.confirmResponse, nil
			}

			opts := targetexecutor.TargetExecutorOptions{
				RunHandler:           mockHandler,
				ConfirmDependency:    mockConfirm,
				DisableArtifactCache: true,
			}

			invocations := map[string]targetexecutor.TargetInvocation{
				input.entryTarget: targetexecutor.TargetInvocationWithScalars(input.parameters),
			}

			// 4. Execute the pipeline
			err := interpreter.HelmInterpreterExecuteTarget(res, ctx, input.entryTarget, invocations, opts)

			return executionScenarioOutput{
				executedCommands: executed,
				pipelineErr:      err,
			}, nil
		},
	)

	return shield.SHIELD_Testing_OperationRunScenario(scenario, execCtx, standardRunCfg)
}

type cacheScenarioInput struct {
	entryTarget string
	bypassCache bool
	mutateInput bool
	secondPass  bool
}

type cacheScenarioOutput struct {
	executedCommands []string
	pipelineErr      error
}

func runCacheScenario(
	execCtx shield.SHIELD_Testing_ExecutionContext,
	sharedHelm *interpreter.HelmInterpreter,
	casesDir string,
) shield.SHIELD_Testing_ScenarioRunResult {
	scenario := shield.SHIELD_Testing_ScenarioCreate(
		"scenario_helm_cache",
		"Validates artifact cache hit, bypass, dependency invalidation, and volatile targets",
		[]shield.SHIELD_Testing_Guard[cacheScenarioInput, cacheScenarioOutput]{
			shield.SHIELD_Testing_GuardCreate(
				"guard_cache_hit_skips_run",
				cacheScenarioInput{entryTarget: "cache_downstream", secondPass: true},
				shield.SHIELD_Testing_GuardPolicyPredicate(func(out cacheScenarioOutput) (bool, string) {
					if out.pipelineErr != nil {
						return false, fmt.Sprintf("expected success, got: %v", out.pipelineErr)
					}
					if containsString(out.executedCommands, "cmd_cache_upstream") {
						return false, "upstream ran on cache hit pass"
					}
					if containsString(out.executedCommands, "cmd_cache_downstream") {
						return false, "downstream ran on cache hit pass"
					}
					return true, ""
				}),
			),
			shield.SHIELD_Testing_GuardCreate(
				"guard_bypass_cache_reruns",
				cacheScenarioInput{entryTarget: "cache_upstream", bypassCache: true, secondPass: true},
				shield.SHIELD_Testing_GuardPolicyPredicate(func(out cacheScenarioOutput) (bool, string) {
					if out.pipelineErr != nil {
						return false, fmt.Sprintf("expected success, got: %v", out.pipelineErr)
					}
					if !containsString(out.executedCommands, "cmd_cache_upstream") {
						return false, "bypass cache did not rerun upstream"
					}
					return true, ""
				}),
			),
			shield.SHIELD_Testing_GuardCreate(
				"guard_dep_output_invalidation",
				cacheScenarioInput{entryTarget: "cache_downstream", mutateInput: true, secondPass: true},
				shield.SHIELD_Testing_GuardPolicyPredicate(func(out cacheScenarioOutput) (bool, string) {
					if out.pipelineErr != nil {
						return false, fmt.Sprintf("expected success, got: %v", out.pipelineErr)
					}
					if !containsString(out.executedCommands, "cmd_cache_upstream") {
						return false, "upstream did not rerun after input mutation"
					}
					if !containsString(out.executedCommands, "cmd_cache_downstream") {
						return false, "downstream did not rerun after upstream invalidation"
					}
					return true, ""
				}),
			),
			shield.SHIELD_Testing_GuardCreate(
				"guard_cache_volatile_always_runs",
				cacheScenarioInput{entryTarget: "cache_volatile", secondPass: true},
				shield.SHIELD_Testing_GuardPolicyPredicate(func(out cacheScenarioOutput) (bool, string) {
					if out.pipelineErr != nil {
						return false, fmt.Sprintf("expected success, got: %v", out.pipelineErr)
					}
					if !containsString(out.executedCommands, "cmd_cache_volatile") {
						return false, "volatile target did not run on second pass"
					}
					return true, ""
				}),
			),
		},
		func(input cacheScenarioInput) (cacheScenarioOutput, error) {
			helmPath := filepath.Join(casesDir, "cache.helm")
			fixtureDir := filepath.Join(casesDir, "cache_fixtures")
			cacheRoot := filepath.Join(casesDir, ".helm_cache_test")

			if err := os.RemoveAll(cacheRoot); err != nil {
				return cacheScenarioOutput{}, err
			}

			if err := cacheScenarioWriteFixtureFiles(fixtureDir); err != nil {
				return cacheScenarioOutput{}, err
			}

			dispatcher := signal.SignalDispatcherCreate(helmExecutionSignalManifest)
			ctx := signal.SignalContextCreate(dispatcher)

			res := interpreter.HelmInterpreterInterpretFile(sharedHelm, helmPath, ctx)
			if res.Error != nil {
				return cacheScenarioOutput{pipelineErr: res.Error}, nil
			}

			var mu sync.Mutex
			var executed []string

			mockHandler := func(req targetexecutor.TargetRunRequest) (targetexecutor.TargetRunResult, error) {
				mu.Lock()
				executed = append(executed, req.Command)
				mu.Unlock()
				return targetexecutor.TargetRunResult{}, nil
			}

			opts := targetexecutor.TargetExecutorOptions{
				RunHandler:  mockHandler,
				CacheRoot:   cacheRoot,
				BypassCache: input.bypassCache,
			}

			invocations := map[string]targetexecutor.TargetInvocation{
				input.entryTarget: {},
			}

			if err := interpreter.HelmInterpreterExecuteTarget(res, ctx, input.entryTarget, invocations, opts); err != nil {
				return cacheScenarioOutput{pipelineErr: err}, nil
			}

			if !input.secondPass {
				return cacheScenarioOutput{executedCommands: executed}, nil
			}

			if input.mutateInput {
				inputPath := filepath.Join(fixtureDir, "input.txt")
				if err := os.WriteFile(inputPath, []byte("mutated-input\n"), 0o644); err != nil {
					return cacheScenarioOutput{}, err
				}
			}

			mu.Lock()
			executed = nil
			mu.Unlock()

			if err := interpreter.HelmInterpreterExecuteTarget(res, ctx, input.entryTarget, invocations, opts); err != nil {
				return cacheScenarioOutput{pipelineErr: err}, nil
			}

			return cacheScenarioOutput{executedCommands: executed}, nil
		},
	)

	return shield.SHIELD_Testing_OperationRunScenario(scenario, execCtx, standardRunCfg)
}

func cacheScenarioWriteFixtureFiles(fixtureDir string) error {
	files := map[string]string{
		"input.txt":    "seed-input\n",
		"out.txt":      "seed-output\n",
		"down_out.txt": "seed-downstream\n",
	}
	for name, content := range files {
		path := filepath.Join(fixtureDir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func containsString(slice []string, target string) bool {
	for _, s := range slice {
		if strings.Contains(s, target) {
			return true
		}
	}
	return false
}

// ------------------------------------------------------------------ PATH RESOLUTION

func helmLSpecPathResolve() (string, func(), error) {
	return helm.ResolveHelmLSpecPath()
}

func helmTestsCasesDirResolve() (string, error) {
	if p := os.Getenv("HELM_TEST_CASES_DIR"); p != "" {
		st, err := os.Stat(p)
		if err != nil {
			return "", fmt.Errorf("HELM_TEST_CASES_DIR not usable (%q): %w", p, err)
		}
		if !st.IsDir() {
			return "", fmt.Errorf("HELM_TEST_CASES_DIR is not a directory: %q", p)
		}
		return p, nil
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	path, err := findProjectFileUpwards(dir, "tools/helm/tests/cases")
	if err != nil {
		return "", fmt.Errorf("%w; set HELM_TEST_CASES_DIR", err)
	}

	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		return "", fmt.Errorf("resolved path is not a directory: %q", path)
	}

	return path, nil
}

func findProjectFileUpwards(startDir, relativePath string) (string, error) {
	dir := startDir

	for {
		candidate := filepath.Join(dir, relativePath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("could not find %s from cwd", relativePath)
}
