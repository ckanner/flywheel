#!/bin/bash
# 构建 npm 发布物（v0.22 平台包模式）：交叉编译四平台 → 注入平台包 → 打 5 个 tarball。
# 用法: scripts/build-npm.sh <version> [outdir]   产物: meta + 4 平台包 tarball。
set -euo pipefail
VERSION="${1:?usage: build-npm.sh <version> [outdir]}"
OUTDIR="${2:-dist-npm}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
DATE="$(date +%Y-%m-%d)"

for pair in darwin-arm64 darwin-amd64 linux-arm64 linux-amd64; do
  GOOS=${pair%-*} GOARCH=${pair#*-} CGO_ENABLED=0 \
    go build -trimpath -ldflags "-s -w -X github.com/ckanner/flywheel/internal/build.Version=$VERSION -X github.com/ckanner/flywheel/internal/build.Date=$DATE" \
    -o "npm/platforms/$pair/flywheel" ./cmd/flywheel
  echo "built $pair"
done

mkdir -p "$OUTDIR"
mkdir -p "$OUTDIR"
OUTABS="$(cd "$OUTDIR" && pwd)"
npm pack --pack-destination "$OUTABS" >/dev/null
for pair in darwin-arm64 darwin-amd64 linux-arm64 linux-amd64; do
  (cd "npm/platforms/$pair" && npm pack --pack-destination "$OUTABS" >/dev/null)
done
echo "== 产物 =="
ls "$OUTDIR"
