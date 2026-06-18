package entityexecutor

import (
	"helm/internal/ir"
)

// EntityUsagePropagation maps entity instance keys to merged usage requirements from
// consumers in the active build closure (transitive from roots).
type EntityUsagePropagation map[string]EntityPropertyBag

// EntityBuildUsagePropagation computes usage bags applied when building each
// dependency entity instance. Usage declared on a consumer flows down its dep edges;
// it never applies to the declaring entity's own adapter expansion.
func EntityBuildUsagePropagation(
	builtIR ir.HelmIR,
	roots []EntityInstance,
) EntityUsagePropagation {
	needed := entityInstanceClosure(builtIR, roots)
	if len(needed) == 0 {
		return nil
	}

	out := make(EntityUsagePropagation)

	var walk func(instanceKey string, accumulated EntityPropertyBag)
	walk = func(instanceKey string, accumulated EntityPropertyBag) {
		if _, ok := needed[instanceKey]; !ok {
			return
		}

		if len(accumulated) > 0 {
			if existing, ok := out[instanceKey]; ok {
				out[instanceKey] = entityPropertyBagMerge(existing, accumulated)
			} else {
				out[instanceKey] = entityPropertyBagCopy(accumulated)
			}
		}

		inst, err := entityInstanceFromKey(instanceKey)
		if err != nil {
			return
		}
		entity := builtIR.Entities[entityInstanceBaseKey(inst)]
		childAccum := entityPropertyBagMerge(accumulated, entityUsagePropertyBag(entity))

		deps, depErr := ir.HelmEntityDepsForConfiguration(entity, inst.Configuration)
		if depErr != nil {
			return
		}
		for _, dep := range deps {
			depInst := ir.HelmEntityDepResolveInstance(dep, inst.Configuration)
			walk(entityInstanceKey(depInst), childAccum)
		}
	}

	for _, root := range roots {
		walk(entityInstanceKey(root), EntityPropertyBag{})
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func entityUsagePropertyBag(entity ir.HelmEntity) EntityPropertyBag {
	if len(entity.UsageBag) == 0 {
		return nil
	}

	bag := make(EntityPropertyBag, len(entity.UsageBag))
	for key := range entity.UsageBag {
		fragments := entityUsageBagFragments(entity, key)
		if len(fragments) > 0 {
			bag[key] = fragments
		}
	}
	if len(bag) == 0 {
		return nil
	}
	return bag
}

func entityUsageBagFragments(entity ir.HelmEntity, key string) []string {
	expr, ok := entity.UsageBag[key]
	if !ok || len(expr) == 0 {
		return nil
	}

	var out []string
	for _, element := range expr {
		if element.Kind == ir.StringListLiteral && element.Literal != "" {
			out = append(out, element.Literal)
		}
	}
	return out
}

func entityPropertyBagCopy(bag EntityPropertyBag) EntityPropertyBag {
	if len(bag) == 0 {
		return nil
	}
	out := make(EntityPropertyBag, len(bag))
	for key, fragments := range bag {
		out[key] = append([]string(nil), fragments...)
	}
	return out
}

func entityPropertyBagMerge(into, from EntityPropertyBag) EntityPropertyBag {
	if len(from) == 0 {
		return into
	}
	if into == nil {
		into = make(EntityPropertyBag, len(from))
	}

	for key, fragments := range from {
		seen := make(map[string]struct{}, len(into[key])+len(fragments))
		for _, fragment := range into[key] {
			seen[fragment] = struct{}{}
		}
		for _, fragment := range fragments {
			if _, exists := seen[fragment]; exists {
				continue
			}
			seen[fragment] = struct{}{}
			into[key] = append(into[key], fragment)
		}
	}
	return into
}

// EntityApplyUsageToResolved merges propagated usage into adapter parameters.
// Entity-local params are listed first; usage fragments append so consumer
// requirements (e.g. -D overrides) appear after local -I flags.
func EntityApplyUsageToResolved(
	resolved EntityResolvedParameters,
	usage EntityPropertyBag,
) EntityResolvedParameters {
	if len(usage) == 0 {
		return resolved
	}

	if resolved.StringLists == nil {
		resolved.StringLists = make(map[string][]string)
	}

	for key, fragments := range usage {
		if len(fragments) == 0 {
			continue
		}
		existing := resolved.StringLists[key]
		resolved.StringLists[key] = append(append([]string(nil), existing...), fragments...)
	}

	return resolved
}
