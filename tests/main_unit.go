package tests

import (
	"fmt"
	"foundation/location"
	"helm/interpreter"
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

// The exact mathematical mapping of the 5 deliberate semantic failures
var badSemanticsExpectations = []helmSemanticErrorExpectation{
	{messageContains: "variable 'VERSION' has already been declared"},
	{messageContains: "use of undeclared variable 'UNDEFINED_VAR'"},
	{messageContains: "help text for target 'build' is missing"},
	{messageContains: "artifacts block for target 'build' is missing"},
	{messageContains: "target 'build' has already been declared"},
	{messageContains: "condition references undeclared parameter 'UNKNOWN_PARAM'"},
	{messageContains: "use of undeclared variable 'TAG'"},
	{messageContains: "help text for target 'publish' is missing"},
	{messageContains: "artifacts block for target 'publish' is missing"},
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
	if len(got) != len(want) {
		return false, fmt.Sprintf(
			"expected %d semantic diagnostics, got %d:\n%s",
			len(want), len(got), helmSemanticDiagnosticsFormat(got),
		)
	}

	for _, exp := range want {
		if !helmSemanticDiagnosticMatchesExpectation(got, exp) {
			return false, fmt.Sprintf(
				"missing expected semantic error (message contains %q);\ngot:\n%s",
				exp.messageContains, helmSemanticDiagnosticsFormat(got),
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
		// Outputting L0 until you fix the sourceText injection in IRFromSyntax
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
