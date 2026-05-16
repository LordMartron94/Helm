package tests

import (
	"fmt"
	"helm/interpreter"
	"os"
	"path/filepath"
	"shield"
	"signal"
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
				if out.errorMsg != "" {
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
	errorMsg string
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
					if out.errorMsg == "" {
						return false, "expected a syntax error from malformed file, but interpretation succeeded"
					}
					return true, ""
				}),
			),
		},
		func(input invalidSyntaxScenarioInput) (invalidSyntaxScenarioOutput, error) {
			path := filepath.Join(casesDir, input.fileName)

			dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
				{Label: "ERROR", Weight: 20},
			})
			ctx := signal.SignalContextCreate(dispatcher)

			res := interpreter.HelmInterpreterInterpretFile(sharedHelm, path, ctx)

			if res.Error != nil {
				return invalidSyntaxScenarioOutput{errorMsg: res.Error.Error()}, nil
			}

			return invalidSyntaxScenarioOutput{errorMsg: ""}, nil
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
