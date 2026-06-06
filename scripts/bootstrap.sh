#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HELM_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

INSTALL_DEST="${INSTALL_DEST:-${HOME}/.local/bin/helm}"
HELM_VERSION="${HELM_VERSION:-}"
BUILD_OUTPUT="${BUILD_OUTPUT:-${HELM_ROOT}/bin/helm}"

function usage() {
	cat <<'EOF'
Build and install the helm CLI when it is not on PATH yet.

Usage:
  ./scripts/bootstrap.sh [options]

Options:
  --dest PATH     Install destination (default: $INSTALL_DEST or ~/.local/bin/helm)
  --version VER   Set helm/internal/version.Version link-time value
  --output PATH   Build artifact path (default: tools/helm/bin/helm)
  -h, --help      Show this help

Environment:
  INSTALL_DEST    Same as --dest
  HELM_VERSION    Same as --version

Expects the force monorepo layout (go.work at root with ./tools/helm) or an
equivalent workspace after running scripts/install_dependencies.sh.

After install, enable bash tab completion with: helm completion bash
EOF
}

function find_force_workspace_root() {
	local dir="${HELM_ROOT}"

	while [[ "${dir}" != "/" ]]; do
		if [[ -f "${dir}/go.work" && -d "${dir}/tools/helm/cmd/helm" ]]; then
			echo "${dir}"
			return 0
		fi
		dir="$(dirname "${dir}")"
	done

	return 1
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

workspace_root=""
if ! workspace_root="$(find_force_workspace_root)"; then
	echo "Could not find force workspace root (go.work with tools/helm)." >&2
	echo "Clone dependencies with ./scripts/install_dependencies.sh and configure go.work," >&2
	echo "or run this script from a full force checkout." >&2
	exit 1
fi

if [[ -z "${HELM_VERSION}" ]]; then
	if version_from_file="$(read_version_from_helmfile)"; then
		HELM_VERSION="${version_from_file}"
	else
		HELM_VERSION="dev"
	fi
fi

required_paths=(
	"${workspace_root}/libs/foundation"
	"${workspace_root}/libs/langspec"
	"${workspace_root}/libs/lingua"
	"${workspace_root}/libs/syntaxa"
	"${workspace_root}/libs/lexarch"
	"${workspace_root}/libs/memcore"
	"${workspace_root}/libs/memforge"
	"${workspace_root}/libs/persistence"
	"${workspace_root}/libs/signal"
	"${workspace_root}/libs/splash"
	"${workspace_root}/libs/structarch"
)

missing=0
for path in "${required_paths[@]}"; do
	if [[ ! -d "${path}" ]]; then
		echo "Missing dependency path: ${path}" >&2
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

echo "Workspace:  ${workspace_root}"
echo "Building:   ${BUILD_OUTPUT}"
echo "Installing: ${INSTALL_DEST}"
echo "Version:    ${HELM_VERSION}"
echo

(
	cd "${workspace_root}"
	go build -ldflags "${ldflags}" -o "${BUILD_OUTPUT}" ./tools/helm/cmd/helm
)

install -m 0755 "${BUILD_OUTPUT}" "${INSTALL_DEST}"

echo
echo "Installed helm to ${INSTALL_DEST}"
if ! command -v helm >/dev/null 2>&1; then
	echo "Ensure $(dirname "${INSTALL_DEST}") is on your PATH."
fi

echo "Verify: helm --version"
