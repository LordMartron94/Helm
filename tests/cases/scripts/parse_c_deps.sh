#!/usr/bin/env bash
# Normalizes clang .d output to workspace-relative paths (one per line).
# Paths outside HELM_ROOT are silently dropped.
set -euo pipefail

HELM_ROOT="${1:?helm root required}"
DEPS_IN="${2:?input .d file required}"
DEPS_OUT="${3:?output manifest required}"

: >"$DEPS_OUT"
while IFS= read -r raw; do
	line="$(echo "$raw" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
	[[ -z "$line" || "$line" == \#* ]] && continue
	if [[ "$line" = /* ]]; then
		rel="$(realpath --relative-to="$HELM_ROOT" "$line" 2>/dev/null || true)"
	else
		rel="$line"
	fi
	[[ -z "$rel" || "$rel" == ..* ]] && continue
	echo "$rel" >>"$DEPS_OUT"
done <"$DEPS_IN"
