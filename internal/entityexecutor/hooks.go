package entityexecutor

import (
	"fmt"
	"sort"

	"helm/internal/ir"
)

// EntityAdapterTargetHooks returns workspace targets that must run before entity steps.
func EntityAdapterTargetHooks(builtIR ir.HelmIR, instanceKey string) ([]string, error) {
	entity, inst, err := entityLookupForInstanceKey(builtIR, instanceKey)
	if err != nil {
		return nil, err
	}

	if ir.HelmEntityIsMetadataOnly(entity) {
		return nil, nil
	}

	adapterName, adapterErr := ir.HelmEntityAdapterNameFor(entity, inst.Configuration)
	if adapterErr != nil {
		return nil, fmt.Errorf("entity '%s': %w", instanceKey, adapterErr)
	}

	adapter, ok := builtIR.Adapters[adapterName]
	if !ok {
		return nil, fmt.Errorf("entity '%s': unknown adapter '%s'", instanceKey, adapterName)
	}

	seen := make(map[string]struct{})
	var hooks []string
	for _, phase := range adapter.Phases {
		for _, targetName := range phase.TargetDependsOn {
			if _, exists := seen[targetName]; exists {
				continue
			}
			seen[targetName] = struct{}{}
			hooks = append(hooks, targetName)
		}
	}
	sort.Strings(hooks)
	return hooks, nil
}
