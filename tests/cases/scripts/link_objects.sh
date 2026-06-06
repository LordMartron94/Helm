#!/usr/bin/env bash
set -euo pipefail
OUT_FILE="$1"
shift
clang "$@" -o "$OUT_FILE"
