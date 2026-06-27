package entityexecutor

import (
	"helm/internal/ir"
	"sort"

	"structarch"
)

// EntityExecutionPlan holds topologically ordered entity build phases.
type EntityExecutionPlan struct {
	Phases [][]string
	Order  []string
}

// EntityBuildExecutionPlan topologically sorts configured entity instances by deps.
func EntityBuildExecutionPlan(builtIR ir.HelmIR, roots []EntityInstance) (*EntityExecutionPlan, error) {
	needed := entityInstanceClosure(builtIR, roots)
	if len(needed) == 0 {
		return &EntityExecutionPlan{}, nil
	}

	graph := make(map[string][]string, len(needed))
	for instanceKey := range needed {
		graph[instanceKey] = []string{}
		inst, err := entityInstanceFromKey(instanceKey)
		if err != nil {
			continue
		}
		entity, ok := builtIR.Entities[entityInstanceBaseKey(inst)]
		if !ok {
			continue
		}
		deps, depErr := ir.HelmEntityDepsForConfiguration(entity, inst.Configuration)
		if depErr != nil {
			continue
		}
		for _, dep := range deps {
			depInst := ir.HelmEntityDepResolveInstance(dep, inst.Configuration)
			depKey := entityInstanceKey(depInst)
			if _, ok := needed[depKey]; !ok {
				continue
			}
			graph[instanceKey] = append(graph[instanceKey], depKey)
		}
	}

	phases, err := structarch.STRUCTARCH_DAG_Resolve(graph)
	if err != nil {
		return nil, err
	}

	var order []string
	for _, phase := range phases {
		order = append(order, phase...)
	}
	return &EntityExecutionPlan{
		Phases: phases,
		Order:  order,
	}, nil
}

func entityInstanceClosure(builtIR ir.HelmIR, roots []EntityInstance) map[string]struct{} {
	needed := make(map[string]struct{})
	var walk func(inst EntityInstance)
	walk = func(inst EntityInstance) {
		key := entityInstanceKey(inst)
		if _, seen := needed[key]; seen {
			return
		}
		entity, ok := builtIR.Entities[entityInstanceBaseKey(inst)]
		if !ok {
			return
		}
		needed[key] = struct{}{}
		deps, err := ir.HelmEntityDepsForConfiguration(entity, inst.Configuration)
		if err != nil {
			return
		}
		for _, dep := range deps {
			walk(ir.HelmEntityDepResolveInstance(dep, inst.Configuration))
		}
	}
	for _, root := range roots {
		walk(root)
	}
	return needed
}

// EntityRootsFromTargetDeps collects configured entity instances referenced by a target,
// including entity labels reachable through transitive target depends_on edges.
func EntityRootsFromTargetDeps(builtIR ir.HelmIR, targetName string) []EntityInstance {
	canonical, ok := ir.IRResolveTargetName(builtIR.Targets, targetName)
	if !ok {
		return nil
	}
	entryConfiguration := ir.HelmTargetEntryConfiguration(builtIR, targetName)

	seenTargets := make(map[string]struct{})
	seenEntities := make(map[string]EntityInstance)

	var walkTarget func(targetCanonical string)
	walkTarget = func(targetCanonical string) {
		if _, seen := seenTargets[targetCanonical]; seen {
			return
		}
		seenTargets[targetCanonical] = struct{}{}

		target, ok := builtIR.Targets[targetCanonical]
		if !ok {
			return
		}

		for _, dep := range target.DependsOn {
			if dep.EntityLabel != nil {
				entityDep := ir.HelmEntityDep{
					Label:         *dep.EntityLabel,
					Configuration: dep.EntityConfiguration,
				}
				inst := ir.HelmEntityDepResolveInstance(entityDep, entryConfiguration)
				seenEntities[entityInstanceKey(inst)] = inst
				continue
			}

			depCanonical, exists := ir.IRResolveTargetName(builtIR.Targets, dep.TargetName)
			if !exists {
				continue
			}
			walkTarget(depCanonical)
		}
	}

	walkTarget(canonical)

	if len(seenEntities) == 0 {
		return nil
	}

	roots := make([]EntityInstance, 0, len(seenEntities))
	for _, inst := range seenEntities {
		roots = append(roots, inst)
	}
	sort.Slice(roots, func(i, j int) bool {
		return entityInstanceKey(roots[i]) < entityInstanceKey(roots[j])
	})
	return roots
}

// EntitySortedKeys returns deterministic entity instance keys from a plan.
func EntitySortedKeys(plan *EntityExecutionPlan) []string {
	if plan == nil {
		return nil
	}
	out := append([]string(nil), plan.Order...)
	sort.Strings(out)
	return out
}

// EntityInstanceClosureFromRoots returns all instance keys reachable from roots.
func EntityInstanceClosureFromRoots(builtIR ir.HelmIR, roots []EntityInstance) map[string]struct{} {
	return entityInstanceClosure(builtIR, roots)
}

// EntityLegacyRootsFromLabels converts label-only roots to default-configuration instances.
func EntityLegacyRootsFromLabels(labels []ir.HelmLabel) []EntityInstance {
	if len(labels) == 0 {
		return nil
	}
	out := make([]EntityInstance, len(labels))
	for i, label := range labels {
		configuration := label.Configuration
		if configuration == "" {
			configuration = ir.HelmConfigurationDefaultName
		}
		label.Configuration = ""
		out[i] = EntityInstance{Label: label, Configuration: configuration}
	}
	return out
}
