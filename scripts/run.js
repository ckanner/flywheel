// Copyright (c) 2026 ckanner. SPDX-License-Identifier: UNLICENSED
//
// npm shim: bin target. Resolves the platform-specific Go binary (downloaded
// at install time by install.js), auto-downloading it on first run when
// missing (npx / yarn / pnpm / local monorepo dev — postinstall may not have
// run). Pattern borrowed from lark-cli (github.com/larksuite/cli).
"use strict";

const { execFileSync } = require("child_process");
const fs = require("fs");
const path = require("path");

const ext = process.platform === "win32" ? ".exe" : "";
const bin = path.join(__dirname, "..", "bin", "flywheel" + ext);

// On Windows, a crashed self-update may have left the binary renamed to .old.
// Recover it before proceeding so the CLI remains functional.
const oldBin = bin + ".old";
function restoreOldBinary() {
  try {
    if (fs.existsSync(bin)) fs.rmSync(bin, { force: true });
    fs.renameSync(oldBin, bin);
    return true;
  } catch (_) {
    return false;
  }
}

if (process.platform === "win32" && fs.existsSync(oldBin)) {
  if (!fs.existsSync(bin)) {
    restoreOldBinary();
  } else {
    try {
      execFileSync(bin, ["--version"], { stdio: "ignore", timeout: 10000 });
      try {
        fs.rmSync(oldBin, { force: true });
      } catch (_) { /* best-effort cleanup */ }
    } catch (_) {
      restoreOldBinary();
    }
  }
}

const args = process.argv.slice(2);

// Auto-download binary if missing (e.g. npx skipped postinstall).
if (!fs.existsSync(bin)) {
  try {
    execFileSync(process.execPath, [path.join(__dirname, "install.js")], {
      stdio: "inherit",
      env: { ...process.env, FLYWHEEL_CLI_RUN: "true" },
    });
  } catch (_) {
    console.error(
      "\nFailed to auto-install flywheel binary.\n" +
        "To fix, run the install script manually:\n" +
        `  node "${path.join(__dirname, "install.js")}"\n`
    );
    process.exit(1);
  }
}

try {
  execFileSync(bin, args, { stdio: "inherit" });
} catch (e) {
  process.exit(e.status || 1);
}
