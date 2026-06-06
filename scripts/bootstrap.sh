#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HELM_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

INSTALL_DEST="${INSTALL_DEST:-${HOME}/.local/bin/helm}"
HELM_VERSION="${HELM_VERSION:-}"
BUILD_OUTPUT="${BUILD_OUTPUT:-${HELM_ROOT}/bin/helm}"
WORKSPACE_ROOT="${WORKSPACE_ROOT:-}"
HELM_MODULE="${HELM_MODULE:-}"

function usage() {
	cat <<'EOF'
Build and install the helm CLI when it is not on PATH yet.

Usage:
  ./scripts/bootstrap.sh [options]

Options:
  --dest PATH       Install destination (default: $INSTALL_DEST or ~/.local/bin/helm)
  --version VER     Set helm/internal/version.Version link-time value
  --output PATH     Build artifact path (default: <helm-root>/bin/helm)
  --workspace PATH  go.work directory (default: auto-detect upward from helm root)
  --module PATH     Helm module path within the workspace (default: auto-detect)
  -h, --help        Show this help

Environment:
  INSTALL_DEST      Same as --dest
  HELM_VERSION      Same as --version
  BUILD_OUTPUT      Same as --output
  WORKSPACE_ROOT    Same as --workspace
  HELM_MODULE       Same as --module (e.g. . or tools/helm)

Expects a go.work file that lists helm and its library dependencies. Typical layouts:
  - helm checkout with deps beside it (go.work at helm root, use . and ./deps/...)
  - monorepo with helm under tools/helm (go.work at repo root, use ./tools/helm and ./libs/...)

After cloning dependencies, run scripts/install_dependencies.sh and configure go.work,
then run this script from the helm checkout.

After install, enable bash tab completion with: helm completion bash
EOF
}

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

function normalize_module_path() {
	local path="$1"

	path="${path#./}"
	path="${path%/}"
	if [[ -z "${path}" ]]; then
		echo "."
	else
		echo "${path}"
	fi
}

function resolve_helm_module() {
	local workspace_root="$1"

	if [[ -n "${HELM_MODULE}" ]]; then
		normalize_module_path "${HELM_MODULE}"
		return 0
	fi

	if [[ ! -f "${HELM_ROOT}/cmd/helm/main.go" ]]; then
		return 1
	fi

	local helm_rel
	helm_rel="$(realpath --relative-to="${workspace_root}" "${HELM_ROOT}")"
	normalize_module_path "${helm_rel}"
}

function helm_build_package() {
	local helm_module="$1"

	if [[ "${helm_module}" == "." ]]; then
		echo "./cmd/helm"
	else
		echo "./${helm_module}/cmd/helm"
	fi
}

function go_work_use_paths() {
	local workspace_root="$1"
	local go_work="${workspace_root}/go.work"
	local in_use=0

	while IFS= read -r line || [[ -n "${line}" ]]; do
		line="${line%%#*}"
		line="${line#"${line%%[![:space:]]*}"}"
		line="${line%"${line##*[![:space:]]}"}"

		if [[ "${line}" == "use (" ]]; then
			in_use=1
			continue
		fi

		if [[ "${in_use}" -eq 1 ]]; then
			if [[ "${line}" == ")" ]]; then
				break
			fi
			if [[ "${line}" == ./* ]]; then
				normalize_module_path "${line}"
			fi
		fi
	done <"${go_work}"
}

function require_command() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "Required command not found: $1" >&2
		exit 1
	fi
}

function read_version_from_helmfile() {
	local helmfile="${HELM_ROOT}/Helmfile"
	if [[ ! -f "${helmfile}" ]]; then
		return 1
	fi

	local line
	line="$(grep -E '^VERSION[[:space:]]*=' "${helmfile}" | head -n 1 || true)"
	if [[ -z "${line}" ]]; then
		return 1
	fi

	printf '%s' "${line#*=}" | tr -d ' "'
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--dest)
		INSTALL_DEST="$2"
		shift 2
		;;
	--version)
		HELM_VERSION="$2"
		shift 2
		;;
	--output)
		BUILD_OUTPUT="$2"
		shift 2
		;;
	--workspace)
		WORKSPACE_ROOT="$2"
		shift 2
		;;
	--module)
		HELM_MODULE="$2"
		shift 2
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "Unknown option: $1" >&2
		usage >&2
		exit 1
		;;
	esac
done

require_command go
require_command install
require_command realpath

if [[ -z "${WORKSPACE_ROOT}" ]]; then
	if ! WORKSPACE_ROOT="$(find_workspace_root)"; then
		echo "Could not find go.work (searched upward from ${HELM_ROOT})." >&2
		echo "Clone dependencies with ./scripts/install_dependencies.sh, create go.work," >&2
		echo "or pass --workspace PATH / set WORKSPACE_ROOT." >&2
		exit 1
	fi
fi

if [[ ! -f "${WORKSPACE_ROOT}/go.work" ]]; then
	echo "No go.work at ${WORKSPACE_ROOT}" >&2
	exit 1
fi

helm_module=""
if ! helm_module="$(resolve_helm_module "${WORKSPACE_ROOT}")"; then
	echo "Could not resolve helm module within ${WORKSPACE_ROOT}." >&2
	echo "Ensure ${HELM_ROOT}/cmd/helm exists or pass --module PATH / set HELM_MODULE." >&2
	exit 1
fi

build_package="$(helm_build_package "${helm_module}")"

if [[ -z "${HELM_VERSION}" ]]; then
	if version_from_file="$(read_version_from_helmfile)"; then
		HELM_VERSION="${version_from_file}"
	else
		HELM_VERSION="dev"
	fi
fi

mapfile -t use_paths < <(go_work_use_paths "${WORKSPACE_ROOT}")
if [[ "${#use_paths[@]}" -eq 0 ]]; then
	echo "No module paths found in ${WORKSPACE_ROOT}/go.work" >&2
	exit 1
fi

missing=0
for path in "${use_paths[@]}"; do
	if [[ "${path}" == "${helm_module}" ]]; then
		continue
	fi
	if [[ ! -d "${WORKSPACE_ROOT}/${path}" ]]; then
		echo "Missing dependency path: ${WORKSPACE_ROOT}/${path}" >&2
		missing=1
	fi
done

if [[ "${missing}" -ne 0 ]]; then
	echo "Run ./scripts/install_dependencies.sh or init submodules, then ensure go.work includes these modules." >&2
	exit 1
fi

mkdir -p "$(dirname "${BUILD_OUTPUT}")"
mkdir -p "$(dirname "${INSTALL_DEST}")"

ldflags="-X helm/internal/version.Version=${HELM_VERSION} -X helm/internal/version.Commit=bootstrap"

echo "Workspace:  ${WORKSPACE_ROOT}"
echo "Module:     ${helm_module}"
echo "Building:   ${BUILD_OUTPUT}"
echo "Installing: ${INSTALL_DEST}"
echo "Version:    ${HELM_VERSION}"
echo

(
	cd "${WORKSPACE_ROOT}"
	go build -ldflags "${ldflags}" -o "${BUILD_OUTPUT}" "${build_package}"
)

install -m 0755 "${BUILD_OUTPUT}" "${INSTALL_DEST}"

echo
echo "Installed helm to ${INSTALL_DEST}"
if ! command -v helm >/dev/null 2>&1; then
	echo "Ensure $(dirname "${INSTALL_DEST}") is on your PATH."
fi

echo "Verify: helm --version"
