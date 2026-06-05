package ir

import (
	"lingua/helm/artifacts"
	"syntaxa"
)

const (
	ERROR_DUPLICATE_VARIABLE        string = "VAR_001"
	ERROR_UNDECLARED_VARIABLE       string = "VAR_002"
	ERROR_INVALID_VARIABLE_VALUE    string = "VAR_003"
	ERROR_VARIABLE_ARRAY_NOT_SCALAR string = "VAR_004"

	ERROR_DUPLICATE_TARGET           string = "TARGET_001"
	ERROR_UNDECLARED_TARGET          string = "TARGET_002"
	ERROR_DUPLICATE_TARGET_PARAMETER string = "TARGET_003"
	ERROR_DUPLICATE_HELP_TEXT        string = "TARGET_004"
	ERROR_NO_HELP_TEXT               string = "TARGET_005"
	ERROR_DUPLICATE_ALIAS            string = "TARGET_006"
	ERROR_DUPLICATE_ALIASES          string = "TARGET_007"
	ERROR_NO_ARTIFACTS               string = "TARGET_008"
	ERROR_DUPLICATE_ARTIFACTS        string = "TARGET_009"
	ERROR_DUPLICATE_WORKDIR          string = "TARGET_010"
	ERROR_DUPLICATE_ENV              string = "TARGET_011"
	ERROR_DUPLICATE_DEPENDS_ON       string = "TARGET_012"
	ERROR_DUPLICATE_ENV_KEY          string = "TARGET_013"
	ERROR_DUPLICATE_DEPENDENCY       string = "TARGET_014"
	ERROR_UNKNOWN_DEPENDENCY_PARAM   string = "TARGET_015"
	ERROR_MISSING_DEPENDENCY_PARAM   string = "TARGET_016"
	ERROR_DUPLICATE_DEPENDENCY_PARAM string = "TARGET_017"
	ERROR_DUPLICATE_INTERACTIVE      string = "TARGET_018"
	ERROR_INTERACTIVE_MATRIX         string = "TARGET_019"
	ERROR_INVALID_INTERACTIVE        string = "TARGET_020"
	ERROR_DUPLICATE_HIDDEN           string = "TARGET_021"
	ERROR_INVALID_HIDDEN             string = "TARGET_022"
	ERROR_DUPLICATE_DYNAMIC          string = "TARGET_023"

	ERROR_UNDECLARED_PARAMETER string = "COND_001"

	ERROR_INVALID_PATH string = "PATH_001"

	ERROR_INVALID_GLOB         string = "GLOB_001"
	ERROR_UNKNOWN_GLOB_KWARG   string = "GLOB_002"
	ERROR_DUPLICATE_GLOB_KWARG string = "GLOB_003"

	ERROR_DUPLICATE_MATRIX    string = "MATRIX_001"
	ERROR_MATRIX_PARAM_SHADOW string = "MATRIX_002"
	ERROR_EMPTY_MATRIX        string = "MATRIX_003"
)

type HelmConditionType int

const (
	ConditionDefined HelmConditionType = iota
	ConditionNotDefined
	ConditionEquals
	ConditionNotEquals
)

type HelmCondition struct {
	ConditionType HelmConditionType
	Parameter     string
	TargetValue   string
	Runs          []HelmRunCommand
}

type HelmTargetStepKind int

const (
	TargetStepRun HelmTargetStepKind = iota
	TargetStepWhen
)

// HelmRunArgvElement is one argv slot in a native run [ ... ] command.
// Exactly one of Literal or ParamName is set: literals interpolate as scalars; ParamName splices
// a bound parameter's path list (artifact arrays expand to one argv element per path).
type HelmRunArgvElement struct {
	Literal   string
	ParamName string
}

// HelmRunCommand is either a legacy string run (shlex-split at execution) or a native argv template.
type HelmRunCommand struct {
	String string
	Argv   []HelmRunArgvElement
}

func HelmRunCommandIsArgv(command HelmRunCommand) bool {
	return len(command.Argv) > 0
}

func HelmRunCommandLiteral(text string) HelmRunCommand {
	return HelmRunCommand{String: text}
}

type HelmTargetStep struct {
	Kind HelmTargetStepKind
	Run  HelmRunCommand
	When *HelmCondition
}

type HelmIR struct {
	// SourceDirectory is the directory containing the interpreted .helm file.
	SourceDirectory string
	GlobalVariables map[string]HelmGlobalVariable
	Targets         map[string]HelmTarget
	Succeeded       bool
}

type HelmTarget struct {
	Name       string
	Aliases    []string
	HelpText   string
	Parameters []HelmTargetParameter

	WorkDir string
	Env     map[string]string

	DependsOn           []HelmTargetDependency
	DependsOnParamNames []string
	Matrix              *HelmMatrix
	Artifacts           *HelmArtifacts
	Interactive         bool
	Hidden              bool
	Steps               []HelmTargetStep
}

type HelmMatrix struct {
	VariableName string
	Values       []HelmMatrixValue
}

type HelmMatrixValueKind int

const (
	MatrixValueLiteral HelmMatrixValueKind = iota
	MatrixValueGlob
)

type HelmMatrixValue struct {
	Kind    HelmMatrixValueKind
	Literal string
	Glob    *HelmGlob
}

type HelmTargetParameter struct {
	Name           string
	Optional       bool
	DependencyList bool
}

type HelmParameterValueKind int

const (
	HelmParameterScalar HelmParameterValueKind = iota
	HelmParameterGlobalRef
	HelmParameterTargetParamRef
	HelmParameterDependencyList
)

// HelmParameterValue is a dependency or invocation parameter at IR/runtime.
// Scalar values come from string literals; GlobalRef binds a global by name (string or artifact array).
// TargetParamRef forwards a parameter from the depending target (e.g. SOURCE_FILES = SOURCE_FILES).
// DependencyList holds nested depends_on entries (including braced targets with params).
type HelmParameterValue struct {
	Kind            HelmParameterValueKind
	Scalar          string
	GlobalName      string
	TargetParamName string
	Dependencies    []HelmTargetDependency
}

// HelmTargetDependency describes an edge in depends_on.
// Parameters holds values from params { ... }. Each dependency target runs at most once
// per graph execution, so all edges supplying params for the same dependency must agree.
type HelmTargetDependency struct {
	TargetName string
	Optional   bool
	Confirm    bool
	Parameters map[string]HelmParameterValue
	SourceNode *syntaxa.SyntaxaLSTNode[artifacts.Node]
}

type HelmGlob struct {
	BaseDirectory  string
	Includes       []string
	Excludes       []string
	FollowSymlinks bool
	Recursive      bool
	Types          string
}

type HelmArtifactInputKind int

const (
	ArtifactInputString HelmArtifactInputKind = iota
	ArtifactInputGlob
)

type HelmArtifactInput struct {
	Kind    HelmArtifactInputKind
	Literal string
	Glob    *HelmGlob
}

type HelmArtifacts struct {
	Volatile bool
	Inputs   []HelmArtifactInput
	Outputs  []HelmArtifactInput
	// Dynamic holds manifest file paths (one path per line format). Cache evaluation
	// hashes the files listed inside each manifest, not the manifest bytes alone.
	Dynamic []HelmArtifactInput
}
