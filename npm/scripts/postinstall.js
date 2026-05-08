#!/usr/bin/env node

"use strict";

const https = require("https");
const http = require("http");
const fs = require("fs");
const path = require("path");
const os = require("os");
const { execSync } = require("child_process");

const PLATFORM_MAP = {
  "darwin-arm64": "@openscholar/darwin-arm64",
  "darwin-x64": "@openscholar/darwin-x64",
  "linux-x64": "@openscholar/linux-x64",
  "linux-arm64": "@openscholar/linux-arm64",
  "win32-x64": "@openscholar/win32-x64",
  "win32-arm64": "@openscholar/win32-arm64",
};

const GORELEASER_MAP = {
  "darwin-arm64": "openscholar_darwin_arm64.tar.gz",
  "darwin-x64": "openscholar_darwin_amd64.tar.gz",
  "linux-x64": "openscholar_linux_amd64.tar.gz",
  "linux-arm64": "openscholar_linux_arm64.tar.gz",
  "win32-x64": "openscholar_windows_amd64.zip",
  "win32-arm64": "openscholar_windows_arm64.zip",
};

function getBinaryName() {
  return process.platform === "win32" ? "openscholar.exe" : "openscholar";
}

function binaryExists() {
  const platformKey = `${process.platform}-${process.arch}`;
  const pkgName = PLATFORM_MAP[platformKey];

  if (pkgName) {
    try {
      const pkgDir = path.dirname(require.resolve(`${pkgName}/package.json`));
      const bin = path.join(pkgDir, getBinaryName());
      if (fs.existsSync(bin)) {
        return true;
      }
    } catch (_) {
      // Not installed via optional dependency
    }
  }

  // Check fallback location
  const fallback = path.join(
    os.homedir(),
    ".openscholar",
    "bin",
    getBinaryName()
  );
  return fs.existsSync(fallback);
}

function getVersion() {
  const pkg = JSON.parse(
    fs.readFileSync(path.join(__dirname, "..", "package.json"), "utf8")
  );
  return pkg.version;
}

function downloadFile(url) {
  return new Promise((resolve, reject) => {
    const get = url.startsWith("https:") ? https.get : http.get;
    get(url, (res) => {
      if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
        return downloadFile(res.headers.location).then(resolve, reject);
      }
      if (res.statusCode !== 200) {
        return reject(new Error(`Download failed: HTTP ${res.statusCode}`));
      }
      const chunks = [];
      res.on("data", (chunk) => chunks.push(chunk));
      res.on("end", () => resolve(Buffer.concat(chunks)));
      res.on("error", reject);
    }).on("error", reject);
  });
}

async function install() {
  if (binaryExists()) {
    return;
  }

  const platformKey = `${process.platform}-${process.arch}`;
  const archiveName = GORELEASER_MAP[platformKey];

  if (!archiveName) {
    console.warn(
      `Warning: No prebuilt binary available for ${platformKey}. ` +
        "You can build from source with: go install github.com/openscholar/openscholar@latest"
    );
    return;
  }

  const version = getVersion();
  const url = `https://github.com/openscholar/openscholar/releases/download/v${version}/${archiveName}`;

  console.log(`Downloading openscholar v${version} for ${platformKey}...`);

  try {
    const data = await downloadFile(url);

    const destDir = path.join(os.homedir(), ".openscholar", "bin");
    fs.mkdirSync(destDir, { recursive: true });

    const tmpFile = path.join(os.tmpdir(), archiveName);
    fs.writeFileSync(tmpFile, data);

    if (archiveName.endsWith(".tar.gz")) {
      execSync(`tar -xzf "${tmpFile}" -C "${destDir}" ${getBinaryName()}`, {
        stdio: "ignore",
      });
    } else if (archiveName.endsWith(".zip")) {
      // For Windows zip files, use PowerShell or unzip
      if (process.platform === "win32") {
        execSync(
          `powershell -Command "Expand-Archive -Path '${tmpFile}' -DestinationPath '${destDir}' -Force"`,
          { stdio: "ignore" }
        );
      } else {
        execSync(`unzip -o "${tmpFile}" ${getBinaryName()} -d "${destDir}"`, {
          stdio: "ignore",
        });
      }
    }

    // Set executable permission on Unix
    if (process.platform !== "win32") {
      const binPath = path.join(destDir, getBinaryName());
      fs.chmodSync(binPath, 0o755);
    }

    // Clean up
    fs.unlinkSync(tmpFile);

    console.log(`openscholar v${version} installed to ${destDir}`);
  } catch (err) {
    console.warn(
      `Warning: Failed to download openscholar binary: ${err.message}\n` +
        "You can install manually: go install github.com/openscholar/openscholar@latest"
    );
  }
}

install();
