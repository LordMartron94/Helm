package ir

import (
	"fmt"
	"strings"
)

const (
	HelmConfigurationDefaultName = "default"

	ERROR_DUPLICATE_CONFIGURATION         string = "CFG_001"
	ERROR_UNKNOWN_CONFIGURATION           string = "CFG_002"
	ERROR_ENTITY_UNKNOWN_CONFIGURATION    string = "CFG_003"
	ERROR_ENTITY_DUPLICATE_CONFIGURATION  string = "CFG_004"
	ERROR_DUPLICATE_TARGET_CONFIGURATION  string = "TARGET_025"
)

// HelmConfigurationDecl is a named global override profile declared at workspace root.
type HelmConfigurationDecl struct {
	Name    string
	Globals map[string]HelmGlobalVariable
}

// HelmEntityConfiguration is one build variant of an entity (adapter, params, deps).
type HelmEntityConfiguration struct {
	AdapterName string
	Parameters  map[string]HelmParameterValue
	Deps        []HelmEntityDep
}

// HelmEntityDep is one entity dependency edge with an optional configuration override.
type HelmEntityDep struct {
	Label         HelmLabel
	Configuration string
}

// HelmEntityInstanceRef identifies one configured entity instance in the build graph.
type HelmEntityInstanceRef struct {
	Label         HelmLabel
	Configuration string
}

// HelmEntityInstanceKey returns the canonical cache/DAG key for a configured entity.
func HelmEntityInstanceKey(label HelmLabel, configuration string) string {
	base := HelmLabelCanonical(HelmLabel{Path: label.Path, Name: label.Name})
	if configuration == "" || configuration == HelmConfigurationDefaultName {
		return base
	}
	return base + "@" + configuration
}

// HelmEntityInstanceKeyParse splits an instance key into base label key and configuration name.
func HelmEntityInstanceKeyParse(instanceKey string) (baseKey string, configuration string) {
	at := strings.LastIndex(instanceKey, "@")
	if at < 0 {
		return instanceKey, HelmConfigurationDefaultName
	}
	return instanceKey[:at], instanceKey[at+1:]
}

// HelmEntityInstanceRefFromKey parses an instance key into a structured reference.
func HelmEntityInstanceRefFromKey(instanceKey string) (HelmEntityInstanceRef, error) {
	baseKey, configuration := HelmEntityInstanceKeyParse(instanceKey)
	label, err := HelmLabelParseKey(baseKey)
	if err != nil {
		return HelmEntityInstanceRef{}, err
	}
	return HelmEntityInstanceRef{Label: label, Configuration: configuration}, nil
}

// HelmLabelParseKey parses a canonical entity key (//path or //path:name) without @configuration.
func HelmLabelParseKey(key string) (HelmLabel, error) {
	return HelmLabelParse(key)
}

// HelmEntityDepResolveInstance resolves the configured instance for a dependency edge.
func HelmEntityDepResolveInstance(dep HelmEntityDep, parentConfiguration string) HelmEntityInstanceRef {
	configuration := parentConfiguration
	if dep.Configuration != "" {
		configuration = dep.Configuration
	}
	if dep.Label.Configuration != "" {
		configuration = dep.Label.Configuration
	}

	label := dep.Label
	label.Configuration = ""
	if configuration == "" {
		configuration = HelmConfigurationDefaultName
	}
	return HelmEntityInstanceRef{Label: label, Configuration: configuration}
}

// HelmEntityConfigurationFor returns the entity variant for the given configuration name.
// Workspace configuration profiles (android, etc.) select root globals; entities inherit
// their default adapter/params/deps unless they declare an explicit configuration block.
func HelmEntityConfigurationFor(entity HelmEntity, configuration string) (HelmEntityConfiguration, error) {
	if configuration == "" {
		configuration = HelmConfigurationDefaultName
	}

	if len(entity.Configurations) > 0 {
		if variant, ok := entity.Configurations[configuration]; ok {
			return variant, nil
		}
		if configuration != HelmConfigurationDefaultName {
			if variant, ok := entity.Configurations[HelmConfigurationDefaultName]; ok {
				return variant, nil
			}
		}
		return HelmEntityConfiguration{}, fmt.Errorf(
			"entity '%s' has no configuration %q",
			HelmLabelCanonical(entity.Label),
			configuration,
		)
	}

	return HelmEntityConfiguration{
		AdapterName: entity.AdapterName,
		Parameters:  entity.Parameters,
		Deps:        HelmEntityDepsFromLabels(entity.Deps),
	}, nil
}

// HelmEntityDepsFromLabels converts legacy label-only deps to structured deps.
func HelmEntityDepsFromLabels(labels []HelmLabel) []HelmEntityDep {
	if len(labels) == 0 {
		return nil
	}
	out := make([]HelmEntityDep, len(labels))
	for i, label := range labels {
		out[i] = HelmEntityDep{Label: label}
	}
	return out
}

// HelmEntityDepsForConfiguration returns deps for an entity configuration variant.
func HelmEntityDepsForConfiguration(entity HelmEntity, configuration string) ([]HelmEntityDep, error) {
	variant, err := HelmEntityConfigurationFor(entity, configuration)
	if err != nil {
		return nil, err
	}
	return variant.Deps, nil
}

// HelmEntityAdapterNameFor returns the adapter name for an entity configuration variant.
func HelmEntityAdapterNameFor(entity HelmEntity, configuration string) (string, error) {
	variant, err := HelmEntityConfigurationFor(entity, configuration)
	if err != nil {
		return "", err
	}
	return variant.AdapterName, nil
}

// HelmEntityParametersFor returns adapter parameters for an entity configuration variant.
func HelmEntityParametersFor(entity HelmEntity, configuration string) (map[string]HelmParameterValue, error) {
	variant, err := HelmEntityConfigurationFor(entity, configuration)
	if err != nil {
		return nil, err
	}
	return variant.Parameters, nil
}

// HelmTargetEntryConfiguration resolves the active configuration for an entry target.
func HelmTargetEntryConfiguration(builtIR HelmIR, entryTarget string) string {
	canonical, ok := IRResolveTargetName(builtIR.Targets, entryTarget)
	if !ok {
		return HelmConfigurationDefaultName
	}
	target := builtIR.Targets[canonical]
	if target.ConfigurationName == "" {
		return HelmConfigurationDefaultName
	}
	return target.ConfigurationName
}
