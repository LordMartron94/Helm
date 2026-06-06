package entityexecutor

import (
	"fmt"
	"sort"

	"helm/internal/ir"
)

// EntityAdapterTargetHooks returns workspace targets that must run before entity steps.
func EntityAdapterTargetHooks(builtIR ir.HelmIR, entityKey string) ([]string, error) {
	entity, ok := builtIR.Entities[entityKey]
	if !ok {
		return nil, fmt.Errorf("entity '%s' not found", entityKey)
	}

	adapter, ok := builtIR.Adapters[entity.AdapterName]
	if !ok {
		return nil, fmt.Errorf("entity '%s': unknown adapter '%s'", entityKey, entity.AdapterName)
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
