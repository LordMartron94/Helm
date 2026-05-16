package ir

import (
	"lingua/helm/artifacts"
	"maps"
	"slices"
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

type HelmIR struct {
	globalVariables map[string]string
	targets         map[string]HelmTarget

	succeeded bool
}

/*
Targets returns a copy of the helm targets... used primarily for testing/debugging.
*/
func (h *HelmIR) Targets() map[string]HelmTarget {
	return maps.Clone(h.targets)
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

/*
Artifacts returns the target's artifacts... used primarily for testing/debugging.
*/
func (h *HelmTarget) Artifacts() *HelmArtifacts {
	return h.artifacts
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

type HelmGlob struct {
	baseDirectory string
	include       string
	exclude       string
}

func (g *HelmGlob) BaseDirectory() string {
	return g.baseDirectory
}

func (g *HelmGlob) Include() string {
	return g.include
}

func (g *HelmGlob) Exclude() string {
	return g.exclude
}

type HelmArtifactInputKind int

const (
	ArtifactInputString HelmArtifactInputKind = iota
	ArtifactInputGlob
)

type HelmArtifactInput struct {
	kind   HelmArtifactInputKind
	string string
	glob   *HelmGlob
}

func (e *HelmArtifactInput) Kind() HelmArtifactInputKind {
	return e.kind
}

func (e *HelmArtifactInput) String() string {
	return e.string
}

func (e *HelmArtifactInput) Glob() *HelmGlob {
	return e.glob
}

type HelmArtifacts struct {
	volatile bool
	inputs   []HelmArtifactInput
	outputs  []string
}

/*
Inputs returns a copy of the artifact inputs... used primarily for testing/debugging.
*/
func (h *HelmArtifacts) Inputs() []HelmArtifactInput {
	return slices.Clone(h.inputs)
}

/*
Outputs returns a copy of the artifact's outputs... used primarily for testing/debugging.
*/
func (h *HelmArtifacts) Outputs() []string {
	return slices.Clone(h.outputs)
}

func (h *HelmIR) Success() bool {
	return h.succeeded
}
