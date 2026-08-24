// Copyright (c) 2026 ckanner. SPDX-License-Identifier: UNLICENSED
//
// npm postinstall: download the platform-specific Go binary from GitHub
// releases (with mirror fallback), verify the SHA-256 checksum against the
// goreleaser-generated checksums.txt, extract, and install into ./bin.
// Pattern borrowed from lark-cli (github.com/larksuite/cli).
"use strict";

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");
const os = require("os");

const VERSION = require("../package.json").version;
const REPO = "ckanner/flywheel";
const NAME = "flywheel";
const DEFAULT_MIRROR_HOST = "https://registry.npmmirror.com";
// Allowlist gates the *initial* request URL only. curl --location follows
// redirects (capped by --max-redirs 3) without re-checking the target host.
// Acceptable because checksum verification is the primary integrity control.
const ALLOWED_HOSTS = new Set([
  "github.com",
  "objects.githubusercontent.com",
  "registry.npmmirror.com",
]);

const PLATFORM_MAP = { darwin: "darwin", linux: "linux", win32: "windows" };
const ARCH_MAP = { x64: "amd64", arm64: "arm64" };

const platform = PLATFORM_MAP[process.platform];
const arch = ARCH_MAP[process.arch];
if (!platform || !arch) {
  console.error(`flywheel: unsupported platform ${process.platform}/${process.arch}`);
  process.exit(1);
}

const isWindows = process.platform === "win32";
const extension = isWindows ? ".zip" : ".tar.gz";
const archiveName = `${NAME}-${VERSION}-${platform}-${arch}${extension}`;
const GITHUB_URL =
  `https://github.com/${REPO}/releases/download/v${VERSION}/${archiveName}`;

const binDir = path.join(__dirname, "..", "bin");
const dest = path.join(binDir, NAME + (isWindows ? ".exe" : ""));

function joinUrl(base, suffix) {
  return base.replace(/\/+$/, "") + suffix;
}

function getMirrorUrls(env) {
  const binaryPath = `/-/binary/${NAME}/v${VERSION}/${archiveName}`;
  const urls = [];
  const registry = (env.npm_config_registry || "").trim();
  if (registry) {
    try {
      const parsed = new URL(registry);
      if (parsed.protocol === "https:" && parsed.hostname && parsed.hostname !== "registry.npmjs.org") {
        urls.push(joinUrl(parsed.origin + parsed.pathname, binaryPath));
      }
    } catch (_) { /* silently ignore malformed registry */ }
  }
  urls.push(joinUrl(DEFAULT_MIRROR_HOST, binaryPath));
  return urls;
}

function assertAllowedHost(url) {
  const { hostname } = new URL(url);
  if (!ALLOWED_HOSTS.has(hostname)) {
    throw new Error(`Download host not allowed: ${hostname}`);
  }
}

function download(url, destPath) {
  assertAllowedHost(url);
  const args = [
    "--fail", "--location", "--silent", "--show-error",
    "--connect-timeout", "10", "--max-time", "120",
    "--max-redirs", "3",
    "--output", destPath,
    url,
  ];
  execFileSync("curl", args, { stdio: ["ignore", "ignore", "pipe"] });
}

function getExpectedChecksum() {
  const checksumsPath = path.join(__dirname, "..", "checksums.txt");
  if (!fs.existsSync(checksumsPath)) {
    throw new Error(`[SECURITY] checksums.txt not found at ${checksumsPath}`);
  }
  for (const line of fs.readFileSync(checksumsPath, "utf8").split("\n")) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    const idx = trimmed.indexOf("  ");
    if (idx === -1) continue;
    if (trimmed.slice(idx + 2) === archiveName) return trimmed.slice(0, idx);
  }
  throw new Error(`checksums.txt has no entry for ${archiveName}`);
}

function verifyChecksum(file, expected) {
  const actual = execFileSync("shasum", ["-a", "256", file], { encoding: "utf8" })
    .split(/\s+/)[0];
  if (actual !== expected) {
    throw new Error(`checksum mismatch for ${file}: expected ${expected}, got ${actual}`);
  }
}

function install() {
  const urls = [GITHUB_URL, ...getMirrorUrls(process.env)];
  fs.mkdirSync(binDir, { recursive: true });
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "flywheel-"));
  const archivePath = path.join(tmpDir, archiveName);
  try {
    let lastErr;
    let downloaded = false;
    for (const url of urls) {
      try {
        download(url, archivePath);
        downloaded = true;
        break;
      } catch (e) {
        lastErr = e;
      }
    }
    if (!downloaded) throw lastErr;
    verifyChecksum(archivePath, getExpectedChecksum());
    execFileSync("tar", ["-xzf", archivePath, "-C", tmpDir], { stdio: "ignore" });
    fs.copyFileSync(path.join(tmpDir, NAME), dest);
    fs.chmodSync(dest, 0o755);
    console.log(`${NAME} v${VERSION} installed successfully`);
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

install();
