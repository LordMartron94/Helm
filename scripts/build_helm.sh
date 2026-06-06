#!/usr/bin/env bash

set -euo pipefail

VERSION="${1:?version required}"
OUT_REL="${2:?output path required relative to helm root}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HELM_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

function find_workspace_root() {
	local dir="${HELM_ROOT}"

	while [[ "${dir}" != "/" ]]; do
		if [[ -f "${dir}/go.work" ]]; then
			echo "${dir}"
			return 0
		fi
		dir="$(dirname "${dir}")"
	done

	return 1
}

function resolve_go_pkg() {
	local workspace_root="$1"

	if [[ -f "${HELM_ROOT}/cmd/helm/main.go" ]]; then
		local helm_rel
		helm_rel="$(realpath --relative-to="${workspace_root}" "${HELM_ROOT}")"
		helm_rel="${helm_rel#./}"
		helm_rel="${helm_rel%/}"
		if [[ -z "${helm_rel}" || "${helm_rel}" == "." ]]; then
			echo "./cmd/helm"
		else
			echo "./${helm_rel}/cmd/helm"
		fi
		return 0
	fi

	if [[ -f "${workspace_root}/tools/helm/cmd/helm/main.go" ]]; then
		echo "./tools/helm/cmd/helm"
		return 0
	fi

	return 1
}

require_command() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "Required command not found: $1" >&2
		exit 1
	fi
}

require_command go
require_command realpath

workspace_root=""
if ! workspace_root="$(find_workspace_root)"; then
	echo "Could not find go.work (searched upward from ${HELM_ROOT})." >&2
	exit 1
fi

build_pkg=""
if ! build_pkg="$(resolve_go_pkg "${workspace_root}")"; then
	echo "Could not resolve helm package within ${workspace_root}." >&2
	echo "Expected ${HELM_ROOT}/cmd/helm or tools/helm/cmd/helm under the workspace." >&2
	exit 1
fi

out_abs="${HELM_ROOT}/${OUT_REL#./}"
cache_root="${HELM_ROOT}/.helm/cache/go"
mkdir -p "$(dirname "${out_abs}")" "${cache_root}/tmp" "${cache_root}/gocache"

ldflags="-X helm/internal/version.Version=${VERSION} -X helm/internal/version.Commit=embedded"

(
	cd "${workspace_root}"
	export GOTMPDIR="${cache_root}/tmp"
	export GOCACHE="${cache_root}/gocache"
	go build -ldflags "${ldflags}" -o "${out_abs}" "${build_pkg}"
)
