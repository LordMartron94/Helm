package entityexecutor

import (
	"crypto/sha256"
	"encoding/binary"
	"helm/internal/expand"
	"sort"
)

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
