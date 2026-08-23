// Preflight check before a release: the npm package version must match the
// latest git tag (vX.Y.Z), so the postinstall download URL is never 404.
"use strict";

const { execFileSync } = require("child_process");
const pkg = require("../package.json");

let tag;
try {
  tag = execFileSync("git", ["describe", "--tags", "--abbrev=0"], { encoding: "utf8" }).trim();
} catch (_) {
  console.error("release:check: no git tag found — tag the release first (vX.Y.Z)");
  process.exit(1);
}
const version = tag.replace(/^v/, "");
if (version !== pkg.version) {
  console.error(`release:check: package.json version ${pkg.version} != git tag ${tag}`);
  process.exit(1);
}
console.log(`release:check: ok (${tag})`);
