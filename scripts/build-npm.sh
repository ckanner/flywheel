#!/usr/bin/env bash
# Build a single, complete verified candidate in an isolated source snapshot.
set -euo pipefail
VERSION="${1:?usage: build-npm.sh <version> [outdir]}"
OUTDIR="${2:-dist-npm}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
exec node "$REPO_ROOT/scripts/release-build.js" "$VERSION" "$OUTDIR"
