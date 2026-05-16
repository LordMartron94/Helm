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

type HelmIR struct {
	globalVariables map[string]string
	targets         map[string]HelmTarget

	succeeded bool
}

type HelmTarget struct {
	name       string
	aliases    []string
	helpText   string
	parameters []HelmTargetParameter

	workDir string
	env     map[string]string
	runs    []string

	dependsOn    []HelmTargetDependency
	artifacts    *HelmArtifacts
	conditionals []HelmCondition
}

type HelmTargetParameter struct {
	name     string
	optional bool
}

type HelmTargetDependency struct {
	targetName string
	optional   bool
	confirm    bool
	sourceNode *syntaxa.SyntaxaLSTNode[artifacts.Node]
}

type HelmArtifacts struct {
	volatile bool
	inputs   []string
	outputs  []string
}

func (h *HelmIR) Success() bool {
	return h.succeeded
}
