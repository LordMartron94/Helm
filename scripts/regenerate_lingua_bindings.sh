#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HELM_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
LINGUA_HELM="${HELM_ROOT}/deps/lingua/helm"
ARTIFACTS="${LINGUA_HELM}/artifacts"

require_command() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "Required command not found: $1" >&2
		exit 1
	fi
}

find_workspace_root() {
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

sublime_user_packages() {
	local config_home="${XDG_CONFIG_HOME:-"${HOME}/.config"}"
	local packages="${config_home}/sublime-text/Packages/User"

	if [[ -d "${packages}" ]]; then
		echo "${packages}"
		return 0
	fi

	return 1
}

install_sublime_artifacts() {
	local packages="$1"

	if ! cp "${ARTIFACTS}/helm.sublime-syntax" "${packages}/helm.sublime-syntax"; then
		echo "warning: could not install helm.sublime-syntax to ${packages}" >&2
		return 1
	fi
	if ! cp "${ARTIFACTS}/Comments (Helm).tmPreferences" "${packages}/Comments (Helm).tmPreferences"; then
		echo "warning: could not install Comments (Helm).tmPreferences to ${packages}" >&2
		return 1
	fi
	echo "Installed Sublime Text Helm syntax to ${packages}"
}

require_command go

if [[ ! -f "${LINGUA_HELM}/helm.lspec" ]]; then
	echo "Missing Lingua Helm spec: ${LINGUA_HELM}/helm.lspec" >&2
	exit 1
fi

workspace_root=""
if ! workspace_root="$(find_workspace_root)"; then
	echo "Could not find go.work (searched upward from ${HELM_ROOT})." >&2
	exit 1
fi

(
	cd "${LINGUA_HELM}"
	# Follow helm/doc.go directives in order: bindings bootstrap, then full toolchains.
	go generate .
)

echo "Wrote ${ARTIFACTS}/go_bindings.go"
echo "Wrote ${ARTIFACTS}/helm.sublime-syntax"
echo "Wrote ${ARTIFACTS}/Comments (Helm).tmPreferences"

if packages="$(sublime_user_packages)"; then
	install_sublime_artifacts "${packages}" || true
fi
