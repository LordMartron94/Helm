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

// EntityFlattenBags merges property bags from dependency entities in deterministic order.
func EntityFlattenBags(
	builtIR ir.HelmIR,
	deps []ir.HelmLabel,
	key string,
) []string {
	var merged []string
	seen := make(map[string]struct{})

	depKeys := make([]string, len(deps))
	for i, dep := range deps {
		depKeys[i] = ir.HelmLabelCanonical(dep)
	}
	sort.Strings(depKeys)

	for _, depKey := range depKeys {
		entity, ok := builtIR.Entities[depKey]
		if !ok {
			continue
		}
		fragments := entityBagFragments(entity, key)
		for _, fragment := range fragments {
			if _, exists := seen[fragment]; exists {
				continue
			}
			seen[fragment] = struct{}{}
			merged = append(merged, fragment)
		}
	}
	return merged
}

func entityBagFragments(entity ir.HelmEntity, key string) []string {
	expr, ok := entity.InterfaceBag[key]
	if !ok {
		// v1 parity aliases
		switch key {
		case "CPPFLAGS":
			expr = entity.InterfaceBag["C_INCLUDES"]
		case "LDFLAGS":
			expr = entity.InterfaceBag["LD_FLAGS"]
		}
	}
	if len(expr) == 0 {
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
