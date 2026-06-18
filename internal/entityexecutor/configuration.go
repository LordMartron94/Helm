package entityexecutor

import (
	"fmt"

	"helm/internal/ir"
)

// EntityInstance identifies one configured build of an entity in the execution graph.
type EntityInstance = ir.HelmEntityInstanceRef

func entityInstanceKey(inst EntityInstance) string {
	return ir.HelmEntityInstanceKey(inst.Label, inst.Configuration)
}

func entityInstanceBaseKey(inst EntityInstance) string {
	return ir.HelmLabelCanonical(inst.Label)
}

func entityInstanceFromKey(instanceKey string) (EntityInstance, error) {
	return ir.HelmEntityInstanceRefFromKey(instanceKey)
}

func entityLookupForInstanceKey(
	builtIR ir.HelmIR,
	instanceKey string,
) (ir.HelmEntity, EntityInstance, error) {
	inst, err := entityInstanceFromKey(instanceKey)
	if err != nil {
		entity, ok := builtIR.Entities[instanceKey]
		if !ok {
			return ir.HelmEntity{}, EntityInstance{}, fmt.Errorf("entity '%s' not found", instanceKey)
		}
		return entity, EntityInstance{
			Label:         entity.Label,
			Configuration: ir.HelmConfigurationDefaultName,
		}, nil
	}

	entity, ok := builtIR.Entities[entityInstanceBaseKey(inst)]
	if !ok {
		return ir.HelmEntity{}, EntityInstance{}, fmt.Errorf("entity '%s' not found", instanceKey)
	}
	return entity, inst, nil
}
