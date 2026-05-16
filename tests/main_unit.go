package tests

import (
	"fmt"
	"foundation/location"
	"helm/interpreter"
	"helm/ir"
	"helm/shared"
	"os"
	"path/filepath"
	"shield"
	"signal"
	"strings"
)

var standardRunCfg = shield.SHIELD_Testing_ScenarioRunConfig{MaxIterations: 1}

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

	specPath, specErr := helmLSpecPathResolve()
	casesDir, casesErr := helmTestsCasesDirResolve()

	if specErr != nil {
		panic(fmt.Sprintf("helm test setup failed (resolve spec): %v", specErr))
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
	{messageContains: "condition references undeclared parameter 'UNKNOWN_PARAM'"},
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
	extractedOutputs []string
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

					if out.extractedOutputs[0] != expectedPath1 {
						return false, fmt.Sprintf("path 1 mismatch: expected %q, got %q", expectedPath1, out.extractedOutputs[0])
					}

					if out.extractedOutputs[1] != expectedPath2 {
						return false, fmt.Sprintf("path 2 mismatch: expected %q, got %q", expectedPath2, out.extractedOutputs[1])
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
			target, exists := res.BuiltIR.Targets()["compile"]
			if !exists {
				return validPathsScenarioOutput{parseError: fmt.Errorf("target 'compile' not found in IR")}, nil
			}

			if target.Artifacts() == nil {
				return validPathsScenarioOutput{parseError: fmt.Errorf("artifacts block was nil")}, nil
			}

			return validPathsScenarioOutput{
				extractedOutputs: target.Artifacts().Outputs(),
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

					if out.extractedInputs[0].Kind() != ir.ArtifactInputGlob {
						return false, "input 0: expected glob entry"
					}
					glob0 := out.extractedInputs[0].Glob()
					if glob0 == nil {
						return false, "input 0: glob is nil"
					}
					if glob0.BaseDirectory() != "libs" {
						return false, fmt.Sprintf("input 0 base dir: expected %q, got %q", "libs", glob0.BaseDirectory())
					}
					if glob0.Include() != "**/*.go" {
						return false, fmt.Sprintf("input 0 include: expected %q, got %q", "**/*.go", glob0.Include())
					}
					if glob0.Exclude() != "" {
						return false, fmt.Sprintf("input 0 exclude: expected empty, got %q", glob0.Exclude())
					}
					if glob0.FollowSymlinks() {
						return false, "input 0 follow_symlinks: expected default false"
					}
					if !glob0.Recursive() {
						return false, "input 0 recursive: expected default true"
					}
					if glob0.Types() != "files" {
						return false, fmt.Sprintf("input 0 types: expected default %q, got %q", "files", glob0.Types())
					}

					if out.extractedInputs[1].Kind() != ir.ArtifactInputGlob {
						return false, "input 1: expected glob entry"
					}
					glob1 := out.extractedInputs[1].Glob()
					if glob1 == nil {
						return false, "input 1: glob is nil"
					}
					if glob1.BaseDirectory() != "." {
						return false, fmt.Sprintf("input 1 base dir: expected %q, got %q", ".", glob1.BaseDirectory())
					}
					if glob1.Exclude() != "*_test.go" {
						return false, fmt.Sprintf("input 1 exclude: expected %q, got %q", "*_test.go", glob1.Exclude())
					}
					if glob1.Include() != "" {
						return false, fmt.Sprintf("input 1 include: expected empty, got %q", glob1.Include())
					}
					if glob1.FollowSymlinks() || !glob1.Recursive() || glob1.Types() != "files" {
						return false, "input 1: expected default follow_symlinks=false, recursive=true, types=files"
					}

					glob2 := out.extractedInputs[2].Glob()
					if glob2 == nil {
						return false, "input 2: glob is nil"
					}
					if glob2.BaseDirectory() != "vendor" {
						return false, fmt.Sprintf("input 2 base dir: expected %q, got %q", "vendor", glob2.BaseDirectory())
					}
					if !glob2.FollowSymlinks() {
						return false, "input 2 follow_symlinks: expected true"
					}
					if glob2.Recursive() {
						return false, "input 2 recursive: expected false"
					}
					if glob2.Types() != "directories" {
						return false, fmt.Sprintf("input 2 types: expected %q, got %q", "directories", glob2.Types())
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

			target, exists := res.BuiltIR.Targets()["scan"]
			if !exists {
				return validGlobsScenarioOutput{parseError: fmt.Errorf("target 'scan' not found in IR")}, nil
			}

			if target.Artifacts() == nil {
				return validGlobsScenarioOutput{parseError: fmt.Errorf("artifacts block was nil")}, nil
			}

			return validGlobsScenarioOutput{
				extractedInputs: target.Artifacts().Inputs(),
			}, nil
		},
	)

	return shield.SHIELD_Testing_OperationRunScenario(scenario, execCtx, standardRunCfg)
}

// ------------------------------------------------------------------ PATH RESOLUTION

func helmLSpecPathResolve() (string, error) {
	if p := os.Getenv("HELM_LSPEC_PATH"); p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("HELM_LSPEC_PATH not usable (%q): %w", p, err)
		}
		return p, nil
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	path, err := findProjectFileUpwards(dir, "libs/lingua/helm/helm.lspec")
	if err != nil {
		return "", fmt.Errorf("%w; set HELM_LSPEC_PATH", err)
	}

	return path, nil
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
