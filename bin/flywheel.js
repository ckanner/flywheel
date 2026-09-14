#!/usr/bin/env node
// Copyright (c) 2026 ckanner. SPDX-License-Identifier: UNLICENSED
//
// npm bin shim（v0.22 平台包模式，esbuild 先例）：按 platform/arch 解析
// optionalDependencies 里的平台二进制包并透传执行——无 postinstall、无网络
// 下载、无 checksum 步骤（npm 自身的包完整性承担校验）。安装即得二进制，
// 维护者与分用户用法完全一致。
"use strict";

const { spawnSync } = require("child_process");

const PLATFORM_MAP = { darwin: "darwin", linux: "linux" };
const ARCH_MAP = { arm64: "arm64", x64: "amd64" };

const platform = PLATFORM_MAP[process.platform];
const arch = ARCH_MAP[process.arch];
if (!platform || !arch) {
  console.error(`flywheel: unsupported platform ${process.platform}/${process.arch}（支持 darwin/linux × amd64/arm64）`);
  process.exit(1);
}

const pkg = `@ckanner/flywheel-${platform}-${arch}`;
let bin;
try {
  bin = require.resolve(`${pkg}/flywheel`);
} catch (_) {
  console.error(`flywheel: 平台包 ${pkg} 未安装。npm 会按 os/cpu 自动选取 optionalDependencies——`);
  console.error(`  若被 --no-optional 跳过，请去掉该标志重装：npm i -g @ckanner/flywheel`);
  process.exit(1);
}

const result = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });
process.exit(result.status === null ? 1 : result.status);
