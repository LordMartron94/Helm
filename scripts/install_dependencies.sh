#!/usr/bin/env bash

set -euo pipefail

# Repositories required to build and run the helm CLI (from `go list -deps ./tools/helm/cmd/helm`
# in the force workspace). Excludes blaze, statarch, shield, and other force-only modules.
REQUIRED_REPOS=(
	"Autarch"
	"Echo"
	"Essence"
	"Foundation"
	"Langspec"
	"Lexarch"
	"Lingua"
	"Memarch"
	"Memcore"
	"Memforge"
	"Memstruct"
	"Persistence"
	"Signal"
	"Splash"
	"Structarch"
	"Syntaxa"
)

GITHUB_OWNER="LordMartron94"
GITHUB_BASE_URL="https://github.com/${GITHUB_OWNER}"

function sanitize_project_name() {
	local raw_name="$1"
	local lowered
	local sanitized

	lowered="$(printf '%s' "${raw_name}" | tr '[:upper:]' '[:lower:]')"
	sanitized="$(printf '%s' "${lowered}" | sed -E 's/[^a-z0-9._-]+/-/g; s/^-+//; s/-+$//')"

	if [[ -z "${sanitized}" ]]; then
		echo "invalid-project"
		return
	fi

	echo "${sanitized}"
}

if [[ ${#REQUIRED_REPOS[@]} -eq 0 ]]; then
	echo "No repositories configured."
	echo "Edit REQUIRED_REPOS in this script before running it."
	exit 1
fi

echo "Helm dependency installer"
echo "Clones libraries required to build tools/helm/cmd/helm."
echo "Source: ${GITHUB_BASE_URL}"
echo
echo "You also need:"
echo "  - Go 1.25+ (https://go.dev/dl/)"
echo "  - A go.work file listing cloned modules and your helm checkout"
echo "  - gcc/pkg-config if your platform links persistence/sqlite with CGO"
echo

read -r -p "Clone dependencies into directory (relative or absolute): " target_input

if [[ -z "${target_input}" ]]; then
	echo "No directory provided."
	exit 1
fi

if [[ "${target_input}" == ~* ]]; then
	target_input="${HOME}${target_input:1}"
fi

if [[ "${target_input}" = /* ]]; then
	target_dir="${target_input}"
else
	target_dir="$(pwd)/${target_input}"
fi

mkdir -p "${target_dir}"
echo "Using directory: ${target_dir}"
echo

for repo_name in "${REQUIRED_REPOS[@]}"; do
	project_dir_name="$(sanitize_project_name "${repo_name}")"
	repo_url="${GITHUB_BASE_URL}/${repo_name}"
	repo_path="${target_dir}/${project_dir_name}"

	if [[ -d "${repo_path}/.git" ]]; then
		echo "Skipping ${repo_name}: already cloned at ${repo_path}"
		continue
	fi

	echo "Cloning ${repo_url} -> ${repo_path}"
	git clone "${repo_url}" "${repo_path}"
done

echo
echo "Done."
echo
echo "Next steps:"
echo "  1. Place this helm repository beside the cloned libs (or inside a monorepo layout)."
echo "  2. Create go.work with use paths to each module and ./tools/helm (or ./helm)."
echo "  3. Run: ./scripts/bootstrap.sh"
echo
echo "Example go.work fragment (adjust paths to your layout):"
echo
echo "go 1.25"
echo
echo "use ("
for repo_name in "${REQUIRED_REPOS[@]}"; do
	project_dir_name="$(sanitize_project_name "${repo_name}")"
	printf '\t./%s\n' "${project_dir_name}"
done
echo -e '\t./helm  # or ./tools/helm in the force monorepo'
echo ")"
