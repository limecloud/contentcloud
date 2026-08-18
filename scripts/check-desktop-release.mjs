import { readFile } from "node:fs/promises";
import { resolve } from "node:path";

const root = resolve(import.meta.dirname, "..");
const policy = JSON.parse(
  await readFile(resolve(root, "apps/desktop/update-channels.json"), "utf8"),
);
const desktopPackage = JSON.parse(
  await readFile(resolve(root, "apps/desktop/package.json"), "utf8"),
);
const forge = await readFile(
  resolve(root, "apps/desktop/forge.config.ts"),
  "utf8",
);
const workflow = await readFile(
  resolve(root, ".github/workflows/desktop-release.yml"),
  "utf8",
);
const staging = await readFile(
  resolve(root, "scripts/stage-desktop-release.mjs"),
  "utf8",
);
const resolver = await readFile(
  resolve(root, "scripts/resolve-desktop-release.mjs"),
  "utf8",
);

const fail = (message) => {
  console.error(`desktop release check failed: ${message}`);
  process.exitCode = 1;
};

if (policy.schema_version !== "contentcloud.desktop-update-policy/1.0")
  fail("unsupported update policy schema");
if (policy.app_id !== "run.zhongcao.contentcloud.desktop")
  fail("unexpected app id");
if (!["stable", "beta"].includes(policy.default_channel))
  fail("default channel must be stable or beta");
if (
  typeof desktopPackage.description !== "string" ||
  desktopPackage.description.trim() === ""
) {
  fail("desktop package description is required by Linux installers");
}
for (const channel of ["stable", "beta"]) {
  const value = policy.channels?.[channel];
  if (
    !value ||
    typeof value.metadata_path !== "string" ||
    value.requires_signed_artifacts !== true ||
    value.allows_downgrade !== false
  ) {
    fail(`${channel} must use signed, non-downgrade metadata`);
  }
}
for (const target of ["darwin-arm64", "darwin-x64", "win32-x64", "linux-x64"]) {
  const value = policy.targets?.[target];
  if (
    !value ||
    !Array.isArray(value.package_formats) ||
    !Array.isArray(value.update_formats)
  )
    fail(`missing target ${target}`);
}
for (const maker of [
  "MakerSquirrel",
  "MakerZIP",
  "MakerDMG",
  "MakerDeb",
  "MakerRpm",
]) {
  if (!forge.includes(`new ${maker}`))
    fail(`Forge maker ${maker} is not configured`);
}
if (forge.includes("electron-builder"))
  fail("electron-builder must not be introduced beside Electron Forge");
for (const marker of [
  '      - "v*"',
  "workflow_dispatch:",
  "node scripts/resolve-desktop-release.mjs",
  "Verify published source matches release tag",
  "desktop-release-publish-${{ needs.resolve.outputs.tag }}",
  "--config.node-linker=hoisted",
  "Prepare Linux package metadata",
  "electron-forge make",
  "actions/upload-artifact@v4",
  "actions/download-artifact@v4",
  "gh release view",
  "gh release create",
  "gh release upload",
  "mv github-release-assets/checksums.txt github-release-assets/desktop-checksums.txt",
  "CONTENTCLOUD_DESKTOP_SIGN",
  "node-gyp rebuild",
]) {
  if (!workflow.includes(marker))
    fail(`desktop release workflow is missing ${marker}`);
}
if (workflow.includes("desktop-v"))
  fail("desktop-only tags must not diverge from the project v* release");
for (const marker of [
  "published desktop releases must build from",
  "inputSourceRef",
  "releaseTagPattern",
]) {
  if (!resolver.includes(marker))
    fail(`desktop release resolver is missing ${marker}`);
}
for (const marker of [
  "contentcloud.desktop-update-metadata/1.0",
  "sha256",
  "requires verified signing",
  "aggregate",
  "desktop-checksums.txt",
]) {
  if (!staging.includes(marker))
    fail(`desktop release staging is missing ${marker}`);
}

if (process.exitCode) process.exit(process.exitCode);
console.log("desktop release policy and Forge makers are valid");
