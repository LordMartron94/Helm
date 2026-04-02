package tests

import (
	"fmt"
	"helm/interpreter"
	"os"
	"path/filepath"
	"shield"
)

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

	for {
		candidate := filepath.Join(dir, "libs/lingua/helm/helm.lspec")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf(
		"could not find libs/lingua/helm/helm.lspec from cwd; set HELM_LSPEC_PATH",
	)
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

	for {
		candidate := filepath.Join(dir, "tools/helm/tests/cases")
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf(
		"could not find tools/helm/tests/cases from cwd; set HELM_TEST_CASES_DIR",
	)
}

func HelmMainUnit(order int) shield.Unit {
	mainUnit := shield.UnitCreate(order, "Helm")

	var sharedHelm *interpreter.HelmInterpreter
	var casesDir string

	shield.UnitSetSetupAndTeardown(mainUnit,
		func() {
			specPath, err := helmLSpecPathResolve()
			if err != nil {
				panic(fmt.Sprintf("helm unit setup (resolve spec): %v", err))
			}
			sharedHelm, err = interpreter.HelmInterpreterTryCreate(specPath)
			if err != nil {
				panic(fmt.Sprintf("helm unit setup (create interpreter): %v", err))
			}

			casesDir, err = helmTestsCasesDirResolve()
			if err != nil {
				panic(fmt.Sprintf("helm unit setup (resolve test cases dir): %v", err))
			}
		},
		func() {
			if sharedHelm != nil {
				interpreter.HelmInterpreterDestroy(sharedHelm)
				sharedHelm = nil
			}
			casesDir = ""
		},
	)

	creationAtom := shield.AtomCreate(0, "Helm Interpreter Creation", func(_ struct{}) string {
		if sharedHelm == nil {
			return "shared interpreter is nil after unit setup"
		}
		return ""
	})

	shield.AtomRegisterCase(creationAtom, shield.CaseCreate(
		"shared_interpreter_ready",
		struct{}{},
		func(out string) shield.AtomResult {
			if out == "" {
				return *shield.AtomResultSuccessCreate()
			}
			return *shield.AtomResultFailureCreate(out)
		},
	))

	helmInterpretRunner := func(caseFile string) interpreter.HelmInterpreterInterpretationResult {
		path := filepath.Join(casesDir, caseFile)
		return interpreter.HelmInterpreterInterpretFile(sharedHelm, path)
	}

	interpretationAtom := shield.AtomCreate(1, "Helm Interpreting", helmInterpretRunner)
	shield.AtomRegisterCase(interpretationAtom, shield.CaseCreate(
		"empty_program",
		"empty_program.helm",
		func(output interpreter.HelmInterpreterInterpretationResult) shield.AtomResult {
			if output.Error != nil {
				return *shield.AtomResultFailureCreate(output.Error.Error())
			}

			return *shield.AtomResultSuccessCreate()
		},
	))

	shield.UnitRegisterAtom(mainUnit, creationAtom)
	shield.UnitRegisterAtom(mainUnit, interpretationAtom)

	return *mainUnit
}
