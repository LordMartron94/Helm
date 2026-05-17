package ir

import (
	"lingua/helm/artifacts"
	"syntaxa"
)

const (
	ERROR_DUPLICATE_VARIABLE  string = "VAR_001"
	ERROR_UNDECLARED_VARIABLE string = "VAR_002"

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

	ERROR_UNDECLARED_PARAMETER string = "COND_001"

	ERROR_INVALID_PATH string = "PATH_001"

	ERROR_INVALID_GLOB         string = "GLOB_001"
	ERROR_UNKNOWN_GLOB_KWARG   string = "GLOB_002"
	ERROR_DUPLICATE_GLOB_KWARG string = "GLOB_003"
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
	Runs          []string
}

type HelmTargetStepKind int

const (
	TargetStepRun HelmTargetStepKind = iota
	TargetStepWhen
)

type HelmTargetStep struct {
	Kind HelmTargetStepKind
	Run  string
	When *HelmCondition
}

type HelmIR struct {
	// SourceDirectory is the directory containing the interpreted .helm file.
	SourceDirectory string
	GlobalVariables map[string]string
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

	DependsOn []HelmTargetDependency
	Artifacts *HelmArtifacts
	Steps     []HelmTargetStep
}

type HelmTargetParameter struct {
	Name     string
	Optional bool
}

type HelmTargetDependency struct {
	TargetName string
	Optional   bool
	Confirm    bool
	SourceNode *syntaxa.SyntaxaLSTNode[artifacts.Node]
}

type HelmGlob struct {
	BaseDirectory  string
	Include        string
	Exclude        string
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
	Outputs  []string
}
