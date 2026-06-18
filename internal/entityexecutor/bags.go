package entityexecutor

import (
	"crypto/sha256"
	"encoding/binary"
	"helm/internal/expand"
	"helm/internal/ir"
	"sort"
)

// EntityPropertyBag is a flattened key → string fragments map (no types).
type EntityPropertyBag map[string][]string

// EntityFlattenBags merges property-bag fragments from direct dependencies in declaration order.
func EntityFlattenBags(
	builtIR ir.HelmIR,
	deps []ir.HelmLabel,
	key string,
) []string {
	var merged []string

	for _, dep := range deps {
		depKey := ir.HelmLabelCanonical(dep)
		entity, ok := builtIR.Entities[depKey]
		if !ok {
			continue
		}
		merged = append(merged, entityBagFragments(entity, key)...)
	}
	return merged
}

// EntityFlattenBagsClosure merges property-bag fragments from the transitive dependency
// closure in reverse topological order (dependents before dependencies).
func EntityFlattenBagsClosure(
	builtIR ir.HelmIR,
	deps []ir.HelmLabel,
	key string,
) []string {
	plan, err := EntityBuildExecutionPlan(builtIR, deps)
	if err != nil || plan == nil || len(plan.Order) == 0 {
		return EntityFlattenBags(builtIR, deps, key)
	}

	var merged []string
	for i := len(plan.Order) - 1; i >= 0; i-- {
		entity, ok := builtIR.Entities[plan.Order[i]]
		if !ok {
			continue
		}
		merged = append(merged, entityBagFragments(entity, key)...)
	}
	return merged
}

func entityBagFragments(entity ir.HelmEntity, key string) []string {
	expr, ok := entity.InterfaceBag[key]
	if !ok || len(expr) == 0 {
		return nil
	}

	var out []string
	for _, element := range expr {
		if element.Kind == ir.StringListLiteral {
			if element.Literal != "" {
				out = append(out, element.Literal)
			}
		}
	}
	return out
}

// EntityBagFingerprint hashes bag contents for cache keys.
func EntityBagFingerprint(bag EntityPropertyBag) uint64 {
	if len(bag) == 0 {
		return 0
	}
	keys := make([]string, 0, len(bag))
	for key := range bag {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var parts []string
	for _, key := range keys {
		fragments := append([]string(nil), bag[key]...)
		sort.Strings(fragments)
		parts = append(parts, key+"="+expand.JoinPathsForShell(fragments))
	}
	hash := sha256.Sum256([]byte(stringsJoin(parts, "|")))
	return binary.BigEndian.Uint64(hash[:8])
}

func stringsJoin(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += sep + parts[i]
	}
	return out
}
