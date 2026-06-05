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

// EntityBuildExecutionPlan topologically sorts entities by deps labels.
func EntityBuildExecutionPlan(builtIR ir.HelmIR, roots []ir.HelmLabel) (*EntityExecutionPlan, error) {
	needed := entityClosure(builtIR, roots)
	if len(needed) == 0 {
		return &EntityExecutionPlan{}, nil
	}

	graph := make(map[string][]string, len(needed))
	for key := range needed {
		graph[key] = []string{}
		entity := builtIR.Entities[key]
		for _, dep := range entity.Deps {
			depKey := ir.HelmLabelCanonical(dep)
			if _, ok := needed[depKey]; !ok {
				continue
			}
			graph[key] = append(graph[key], depKey)
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

func entityClosure(builtIR ir.HelmIR, roots []ir.HelmLabel) map[string]struct{} {
	needed := make(map[string]struct{})
	var walk func(label ir.HelmLabel)
	walk = func(label ir.HelmLabel) {
		key := ir.HelmLabelCanonical(label)
		if _, seen := needed[key]; seen {
			return
		}
		entity, ok := builtIR.Entities[key]
		if !ok {
			return
		}
		needed[key] = struct{}{}
		for _, dep := range entity.Deps {
			walk(dep)
		}
	}
	for _, root := range roots {
		walk(root)
	}
	return needed
}

// EntityRootsFromTargetDeps collects entity labels referenced by a target's depends_on edges.
func EntityRootsFromTargetDeps(builtIR ir.HelmIR, targetName string) []ir.HelmLabel {
	canonical, ok := ir.IRResolveTargetName(builtIR.Targets, targetName)
	if !ok {
		return nil
	}
	target := builtIR.Targets[canonical]
	var roots []ir.HelmLabel
	for _, dep := range target.DependsOn {
		if dep.EntityLabel != nil {
			roots = append(roots, *dep.EntityLabel)
		}
	}
	return roots
}

// EntitySortedKeys returns deterministic entity keys from a plan.
func EntitySortedKeys(plan *EntityExecutionPlan) []string {
	if plan == nil {
		return nil
	}
	out := append([]string(nil), plan.Order...)
	sort.Strings(out)
	return out
}
