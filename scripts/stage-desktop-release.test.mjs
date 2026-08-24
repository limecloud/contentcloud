import { execFile } from "node:child_process";
import { mkdtemp, mkdir, readFile, readdir, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { promisify } from "node:util";
import assert from "node:assert/strict";

import { aggregateRelease, stageTarget } from "./stage-desktop-release.mjs";

const execFileAsync = promisify(execFile);

async function fixtureTarget(target) {
  const root = await mkdtemp(join(tmpdir(), "contentcloud-desktop-release-"));
  const forgeDir = join(root, "forge");
  const makeDir = join(forgeDir, "make");
  const outDir = join(root, "staged");
  await mkdir(join(makeDir, "dmg", "arm64"), { recursive: true });
  await mkdir(join(makeDir, "zip", "darwin", "arm64"), { recursive: true });
  await writeFile(
    join(makeDir, "dmg", "arm64", "Content Work OS-0.28.0-arm64.dmg"),
    "dmg",
  );
  await writeFile(
    join(
      makeDir,
      "zip",
      "darwin",
      "arm64",
      "Content Work OS-darwin-arm64-0.28.0.zip",
    ),
    "zip",
  );
  return { root, forgeDir, outDir, target };
}

test("stages macOS Forge assets with checksums and verified signing metadata", async () => {
  const fixture = await fixtureTarget("darwin-arm64");
  const metadata = await stageTarget({
    ...fixture,
    version: "0.28.0-beta.1",
    channel: "beta",
    tag: "v0.28.0-beta.1",
    repository: "limecloud/contentcloud",
    signed: true,
  });
  assert.equal(metadata.artifacts.length, 2);
  assert.equal(metadata.signing.status, "verified");
  const checksums = await readFile(
    join(fixture.outDir, "darwin-arm64-checksums.sha256"),
    "utf8",
  );
  assert.match(checksums, /darwin-arm64-Content-Work-OS-0.28.0-arm64\.dmg/);
});

test("rejects unsigned macOS release staging", async () => {
  const fixture = await fixtureTarget("darwin-arm64");
  await assert.rejects(
    stageTarget({
      ...fixture,
      version: "0.28.0",
      channel: "stable",
      tag: "v0.28.0",
      signed: false,
    }),
    /requires verified signing/,
  );
});

test("stages unsigned macOS previews without claiming verified signing", async () => {
  const fixture = await fixtureTarget("darwin-arm64");
  const metadata = await stageTarget({
    ...fixture,
    version: "0.28.0",
    channel: "stable",
    tag: "v0.28.0",
    signed: false,
    preview: true,
  });
  assert.deepEqual(metadata.signing, {
    required: true,
    status: "unverified-preview",
  });
  assert.equal(metadata.artifacts[0].download_url, undefined);
});

test("CLI entrypoint stages Windows preview assets", async () => {
  const root = await mkdtemp(
    join(tmpdir(), "contentcloud-desktop-release-cli-"),
  );
  const forgeDir = join(root, "forge");
  const makeDir = join(forgeDir, "make", "squirrel.windows", "x64");
  const outDir = join(root, "staged");
  await mkdir(makeDir, { recursive: true });
  await writeFile(join(makeDir, "ContentWorkOSSetup.exe"), "exe");
  await writeFile(join(makeDir, "content_work_os-0.28.0-full.nupkg"), "nupkg");
  await writeFile(join(makeDir, "RELEASES"), "releases");

  const { stdout } = await execFileAsync(process.execPath, [
    join(import.meta.dirname, "stage-desktop-release.mjs"),
    "stage",
    "--forge-dir",
    forgeDir,
    "--out-dir",
    outDir,
    "--target",
    "win32-x64",
    "--version",
    "0.28.0",
    "--channel",
    "stable",
    "--tag",
    "v0.28.0",
    "--preview",
  ]);

  const metadata = JSON.parse(stdout);
  assert.equal(metadata.target, "win32-x64");
  assert.equal(metadata.artifacts.length, 3);
  assert.deepEqual((await readdir(outDir)).sort(), [
    "win32-x64-ContentWorkOSSetup.exe",
    "win32-x64-RELEASES",
    "win32-x64-checksums.sha256",
    "win32-x64-content_work_os-0.28.0-full.nupkg",
    "win32-x64-latest.json",
  ]);
});

test("rejects unsigned previews when aggregating a release", async () => {
  const fixture = await fixtureTarget("darwin-arm64");
  await stageTarget({
    ...fixture,
    version: "0.28.0",
    channel: "stable",
    tag: "v0.28.0",
    preview: true,
  });
  await assert.rejects(
    aggregateRelease({
      inputDir: fixture.outDir,
      outDir: join(fixture.root, "release"),
      channel: "stable",
      tag: "v0.28.0",
    }),
    /staged target is not signed/,
  );
});

test("rejects incomplete Forge release output", async () => {
  const fixture = await fixtureTarget("darwin-arm64");
  const incompleteForgeDir = join(fixture.root, "incomplete-forge");
  await mkdir(join(incompleteForgeDir, "make", "dmg"), { recursive: true });
  await writeFile(
    join(incompleteForgeDir, "make", "dmg", "Content Work OS.dmg"),
    "dmg",
  );
  await assert.rejects(
    stageTarget({
      ...fixture,
      forgeDir: incompleteForgeDir,
      version: "0.28.0",
      channel: "stable",
      tag: "v0.28.0",
      signed: true,
    }),
    /incomplete; missing zip/,
  );
});

test("aggregates target metadata into a release index", async () => {
  const fixture = await fixtureTarget("darwin-arm64");
  await stageTarget({
    ...fixture,
    version: "0.28.0-beta.1",
    channel: "beta",
    tag: "v0.28.0-beta.1",
    repository: "limecloud/contentcloud",
    signed: true,
  });
  const aggregateDir = join(fixture.root, "release");
  const index = await aggregateRelease({
    inputDir: fixture.outDir,
    outDir: aggregateDir,
    channel: "beta",
    tag: "v0.28.0-beta.1",
    repository: "limecloud/contentcloud",
  });
  assert.deepEqual(Object.keys(index.targets), ["darwin-arm64"]);
  assert.match(
    await readFile(join(aggregateDir, "desktop-checksums.txt"), "utf8"),
    /desktop-beta-latest\.json/,
  );
});

test("rejects an incomplete release matrix", async () => {
  const fixture = await fixtureTarget("darwin-arm64");
  await stageTarget({
    ...fixture,
    version: "0.28.0",
    channel: "stable",
    tag: "v0.28.0",
    signed: true,
  });
  await assert.rejects(
    aggregateRelease({
      inputDir: fixture.outDir,
      outDir: join(fixture.root, "release"),
      channel: "stable",
      tag: "v0.28.0",
      requireAllTargets: true,
    }),
    /missing targets/,
  );
});

test("rejects a staged artifact integrity mismatch", async () => {
  const fixture = await fixtureTarget("darwin-arm64");
  const metadata = await stageTarget({
    ...fixture,
    version: "0.28.0",
    channel: "stable",
    tag: "v0.28.0",
    signed: true,
  });
  await writeFile(join(fixture.outDir, metadata.artifacts[0].name), "tampered");
  await assert.rejects(
    aggregateRelease({
      inputDir: fixture.outDir,
      outDir: join(fixture.root, "release"),
      channel: "stable",
      tag: "v0.28.0",
    }),
    /integrity mismatch/,
  );
});

test("rejects the retired desktop-only release tag", async () => {
  const fixture = await fixtureTarget("darwin-arm64");
  await assert.rejects(
    stageTarget({
      ...fixture,
      version: "0.28.0",
      channel: "stable",
      tag: "desktop-v0.28.0",
      signed: true,
    }),
    /release tag must be v0\.28\.0/,
  );
});
