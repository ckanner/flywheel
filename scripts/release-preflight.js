// Copyright (c) 2026 ckanner. SPDX-License-Identifier: UNLICENSED
//
// 发布前检查（v0.22 平台包模式）：git tag、meta 包、4 个平台包的版本三者
// 必须一致——optionalDependencies 按版本锁定选取，任何漂移都会让用户装到
// 错配的平台二进制。用法：node scripts/release-preflight.js（npm run release:check）。
"use strict";

const { execFileSync } = require("child_process");
const fs = require("fs");
const path = require("path");

const PLATFORMS = ["darwin-arm64", "darwin-amd64", "linux-arm64", "linux-amd64"];

let tag;
try {
  tag = execFileSync("git", ["describe", "--tags", "--abbrev=0"], { encoding: "utf-8" }).trim();
} catch (_) {
  console.error("release:check: no git tag found — tag the release first (vX.Y.Z)");
  process.exit(1);
}
const tagVersion = tag.replace(/^v/, "");

const meta = require("../package.json");
if (meta.version !== tagVersion) {
  console.error(`release:check: meta package version ${meta.version} != tag ${tagVersion}`);
  process.exit(1);
}

for (const p of PLATFORMS) {
  const pkgPath = path.join(__dirname, "..", "npm", "platforms", p, "package.json");
  if (!fs.existsSync(pkgPath)) {
    console.error(`release:check: missing platform manifest ${pkgPath}`);
    process.exit(1);
  }
  const v = JSON.parse(fs.readFileSync(pkgPath, "utf-8")).version;
  if (v !== tagVersion) {
    console.error(`release:check: @ckanner/flywheel-${p} version ${v} != tag ${tagVersion}`);
    process.exit(1);
  }
  const dep = (meta.optionalDependencies || {})[`@ckanner/flywheel-${p}`];
  if (dep !== tagVersion) {
    console.error(`release:check: meta optionalDependencies pin for ${p} is ${dep}, expected ${tagVersion}`);
    process.exit(1);
  }
}

console.log(`release:check: ok (v${tagVersion}: tag = meta = ${PLATFORMS.length} platform packages)`);
