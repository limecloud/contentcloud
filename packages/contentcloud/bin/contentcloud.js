#!/usr/bin/env node

import { createHash } from 'node:crypto';
import { existsSync } from 'node:fs';
import { chmod, mkdir, readFile, rename, rm, writeFile } from 'node:fs/promises';
import { homedir, platform, arch } from 'node:os';
import { join } from 'node:path';
import { spawn } from 'node:child_process';
import { gunzipSync } from 'node:zlib';
import process from 'node:process';

const packageJSON = JSON.parse(await readFile(new URL('../package.json', import.meta.url), 'utf8'));
const version = packageJSON.version;
const platformNames = {darwin: 'darwin', linux: 'linux', win32: 'windows'};
const archNames = {x64: 'amd64', arm64: 'arm64'};
const targetPlatform = platformNames[platform()];
const targetArch = archNames[arch()];

if (!targetPlatform || !targetArch) {
  fail(`Unsupported platform: ${platform()}/${arch()}`);
}

const extension = targetPlatform === 'windows' ? '.exe' : '';
const fileName = `contentcloud-${targetPlatform}-${targetArch}${extension}.gz`;
const installDir = process.env.CONTENTCLOUD_INSTALL_DIR || join(homedir(), '.contentcloud', 'bin');
const binaryPath = join(installDir, `contentcloud-${version}${extension}`);
const explicitBinary = process.env.CONTENTCLOUD_BINARY_PATH;

try {
  const args = process.argv.slice(2);
  if (args[0] === 'update') {
    await install(true);
    process.stdout.write(`ContentCloud ${version} installed at ${binaryPath}\n`);
    process.exit(0);
  }
  const executable = explicitBinary || await install(false);
  const child = spawn(executable, args, {stdio: 'inherit', env: process.env});
  child.on('error', error => fail(error.message));
  child.on('exit', (code, signal) => {
    if (signal) process.kill(process.pid, signal);
    process.exit(code ?? 1);
  });
} catch (error) {
  fail(error instanceof Error ? error.message : String(error));
}

async function install(force) {
  if (explicitBinary) {
    if (!existsSync(explicitBinary)) throw new Error(`CONTENTCLOUD_BINARY_PATH does not exist: ${explicitBinary}`);
    return explicitBinary;
  }
  if (!force && existsSync(binaryPath)) return binaryPath;
  const base = (process.env.CONTENTCLOUD_DOWNLOAD_BASE_URL || `https://github.com/limecloud/contentcloud/releases/download/v${version}`).replace(/\/$/, '');
  const checksumURL = `${base}/checksums.txt`;
  const artifactURL = `${base}/${fileName}`;
  const [checksums, compressed] = await Promise.all([download(checksumURL), download(artifactURL)]);
  const expected = checksumFor(checksums.toString('utf8'), fileName);
  const actual = createHash('sha256').update(compressed).digest('hex');
  if (actual !== expected) throw new Error(`Checksum mismatch for ${fileName}`);
  const binary = gunzipSync(compressed);
  await mkdir(installDir, {recursive: true, mode: 0o700});
  const temporary = `${binaryPath}.tmp-${process.pid}`;
  await writeFile(temporary, binary, {mode: 0o700});
  await chmod(temporary, 0o700);
  await rename(temporary, binaryPath).catch(async error => {
    await rm(temporary, {force: true});
    throw error;
  });
  return binaryPath;
}

async function download(url) {
  const response = await fetch(url, {redirect: 'follow', headers: {'User-Agent': `contentcloud-installer/${version}`}});
  if (!response.ok) throw new Error(`Download failed (${response.status}): ${url}`);
  const requested = new URL(url);
  const final = new URL(response.url);
  const configured = process.env.CONTENTCLOUD_DOWNLOAD_BASE_URL;
  if (!configured && final.hostname !== 'github.com' && final.hostname !== 'objects.githubusercontent.com' && !final.hostname.endsWith('.githubusercontent.com')) {
    throw new Error(`Unexpected download host: ${final.hostname}`);
  }
  if (configured && final.origin !== requested.origin) throw new Error(`Download redirected outside configured origin: ${final.origin}`);
  return Buffer.from(await response.arrayBuffer());
}

function checksumFor(manifest, name) {
  for (const line of manifest.split(/\r?\n/)) {
    const match = line.trim().match(/^([a-f0-9]{64})\s+\*?(.+)$/i);
    if (match && match[2] === name) return match[1].toLowerCase();
  }
  throw new Error(`No checksum found for ${name}`);
}

function fail(message) {
  process.stderr.write(`contentcloud installer: ${message}\n`);
  process.exit(1);
}
