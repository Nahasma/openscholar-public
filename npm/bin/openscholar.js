#!/usr/bin/env node

"use strict";

const { execFileSync } = require("child_process");
const os = require("os");
const path = require("path");
const fs = require("fs");

const PLATFORM_MAP = {
  "darwin-arm64": "@openscholar/darwin-arm64",
  "darwin-x64": "@openscholar/darwin-x64",
  "linux-x64": "@openscholar/linux-x64",
  "linux-arm64": "@openscholar/linux-arm64",
  "win32-x64": "@openscholar/win32-x64",
  "win32-arm64": "@openscholar/win32-arm64",
};

function getBinaryName() {
  return process.platform === "win32" ? "openscholar.exe" : "openscholar";
}

function findBinary() {
  const platformKey = `${process.platform}-${process.arch}`;
  const pkgName = PLATFORM_MAP[platformKey];

  if (pkgName) {
    try {
      const pkgDir = path.dirname(require.resolve(`${pkgName}/package.json`));
      const bin = path.join(pkgDir, getBinaryName());
      if (fs.existsSync(bin)) {
        return bin;
      }
    } catch (_) {
      // Package not installed, fall through
    }
  }

  // Fallback: check ~/.openscholar/bin/
  const fallback = path.join(
    os.homedir(),
    ".openscholar",
    "bin",
    getBinaryName()
  );
  if (fs.existsSync(fallback)) {
    return fallback;
  }

  console.error(
    `Error: Could not find openscholar binary for ${process.platform}-${process.arch}.\n` +
      "Try reinstalling: npm install -g openscholar"
  );
  process.exit(1);
}

const bin = findBinary();

try {
  execFileSync(bin, process.argv.slice(2), { stdio: "inherit" });
} catch (err) {
  if (err.status !== undefined) {
    process.exit(err.status);
  }
  throw err;
}
