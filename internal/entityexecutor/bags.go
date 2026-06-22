package entityexecutor

import (
	"helm/internal/ir"
)

// EntityPropertyBag is a flattened key → string fragments map (no types).
type EntityPropertyBag map[string][]string

// EntityFlattenBags merges property-bag fragments from direct dependencies in declaration order.
func EntityFlattenBags(
	builtIR ir.HelmIR,
	deps []ir.HelmEntityDep,
	parentConfiguration string,
	key string,
) []string {
	var merged []string

	for _, dep := range deps {
		depKey := entityInstanceBaseKey(ir.HelmEntityDepResolveInstance(dep, parentConfiguration))
		entity, ok := builtIR.Entities[depKey]
		if !ok {
			continue
		}
		merged = append(merged, entityBagFragments(builtIR.SourceDirectory, entity, key)...)
	}
	return merged
}

// EntityFlattenBagsClosure merges property-bag fragments from the transitive dependency
// closure in reverse topological order (dependents before dependencies).
func EntityFlattenBagsClosure(
	builtIR ir.HelmIR,
	deps []ir.HelmEntityDep,
	parentConfiguration string,
	key string,
) []string {
	roots := make([]EntityInstance, 0, len(deps))
	for _, dep := range deps {
		roots = append(roots, ir.HelmEntityDepResolveInstance(dep, parentConfiguration))
	}

	plan, err := EntityBuildExecutionPlan(builtIR, roots)
	if err != nil || plan == nil || len(plan.Order) == 0 {
		return EntityFlattenBags(builtIR, deps, parentConfiguration, key)
	}

	var merged []string
	for i := len(plan.Order) - 1; i >= 0; i-- {
		inst, instErr := entityInstanceFromKey(plan.Order[i])
		if instErr != nil {
			continue
		}
		entity, ok := builtIR.Entities[entityInstanceBaseKey(inst)]
		if !ok {
			continue
		}
		merged = append(merged, entityBagFragments(builtIR.SourceDirectory, entity, key)...)
	}
	return merged
}

func entityBagFragments(workspaceRoot string, entity ir.HelmEntity, key string) []string {
	expr, ok := entity.InterfaceBag[key]
	if !ok || len(expr) == 0 {
		return nil
	}

	declarationBase := EntityDeclarationBaseDir(workspaceRoot, entity)
	var out []string
	for _, element := range expr {
		switch element.Kind {
		case ir.StringListLiteral:
			if element.Literal != "" {
				out = append(out, element.Literal)
			}
		case ir.StringListRel:
			if element.RelPath != "" {
				out = append(out, EntityPathsToWorkspace(workspaceRoot, declarationBase, []string{element.RelPath})...)
			}
		}
	}
	return out
}
