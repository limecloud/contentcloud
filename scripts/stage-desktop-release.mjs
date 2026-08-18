import { createHash } from "node:crypto";
import {
  mkdir,
  readdir,
  readFile,
  stat,
  writeFile,
  copyFile,
} from "node:fs/promises";
import { basename, extname, join, relative, resolve } from "node:path";
import { pathToFileURL } from "node:url";

const metadataSchema = "contentcloud.desktop-update-metadata/1.0";
const indexSchema = "contentcloud.desktop-update-index/1.0";

const targetDefinitions = {
  "darwin-arm64": {
    platform: "darwin",
    arch: "arm64",
    formats: ["dmg", "zip"],
    requiredAssets: ["dmg", "zip"],
    signed: true,
  },
  "darwin-x64": {
    platform: "darwin",
    arch: "x64",
    formats: ["dmg", "zip"],
    requiredAssets: ["dmg", "zip"],
    signed: true,
  },
  "win32-x64": {
    platform: "win32",
    arch: "x64",
    formats: ["squirrel"],
    requiredAssets: ["exe", "nupkg", "RELEASES"],
    signed: true,
  },
  "linux-x64": {
    platform: "linux",
    arch: "x64",
    formats: ["deb", "rpm"],
    requiredAssets: ["deb", "rpm"],
    signed: false,
  },
};

const describeAsset = (file, target) => {
  const extension = extname(file).toLowerCase().replace(/^\./, "");
  if (target === "win32-x64") {
    if (extension === "exe") return { format: "squirrel", kind: "exe" };
    if (extension === "nupkg") return { format: "squirrel", kind: "nupkg" };
    if (basename(file).toUpperCase() === "RELEASES")
      return { format: "squirrel", kind: "RELEASES" };
  }
  if (target.startsWith("darwin-") && ["dmg", "zip"].includes(extension))
    return { format: extension, kind: extension };
  if (target === "linux-x64" && ["deb", "rpm"].includes(extension))
    return { format: extension, kind: extension };
  return undefined;
};

const normalizeArtifactName = (target, file) =>
  `${target}-${basename(file).replace(/[^A-Za-z0-9._-]+/g, "-")}`;

async function filesUnder(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) {
      files.push(...(await filesUnder(path)));
    } else if (entry.isFile()) {
      files.push(path);
    }
  }
  return files.sort();
}

async function ensureEmptyDirectory(directory) {
  await mkdir(directory, { recursive: true });
  const entries = await readdir(directory);
  if (entries.length > 0) {
    throw new Error(`output directory must be empty: ${directory}`);
  }
}

async function sha256(path) {
  const content = await readFile(path);
  return createHash("sha256").update(content).digest("hex");
}

function parseArgs(argv) {
  const args = {};
  for (let index = 0; index < argv.length; index += 1) {
    const token = argv[index];
    if (!token.startsWith("--")) continue;
    const key = token.slice(2);
    const next = argv[index + 1];
    if (!next || next.startsWith("--")) {
      args[key] = true;
    } else {
      args[key] = next;
      index += 1;
    }
  }
  return args;
}

function required(args, key) {
  const value = args[key];
  if (typeof value !== "string" || value.trim() === "") {
    throw new Error(`missing --${key}`);
  }
  return value.trim();
}

function validateReleaseIdentity({ version, channel, tag }) {
  if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(version)) {
    throw new Error(`invalid release version: ${version}`);
  }
  if (!["stable", "beta"].includes(channel)) {
    throw new Error(`invalid release channel: ${channel}`);
  }
  if (tag !== `v${version}`) {
    throw new Error(`release tag must be v${version}: ${tag}`);
  }
}

async function stageTarget({
  forgeDir,
  outDir,
  target,
  version,
  channel,
  tag,
  repository,
  signed,
  preview,
}) {
  const definition = targetDefinitions[target];
  if (!definition) throw new Error(`unsupported desktop target: ${target}`);
  validateReleaseIdentity({ version, channel, tag });
  if (signed === true && preview === true) {
    throw new Error("desktop target cannot be both signed and preview-only");
  }
  if (definition.signed && signed !== true && preview !== true) {
    throw new Error(
      `${target} requires verified signing before it can enter ${channel}`,
    );
  }

  const makeDir = join(forgeDir, "make");
  const sourceFiles = await filesUnder(makeDir);
  const assets = sourceFiles
    .map((file) => ({ file, ...describeAsset(file, target) }))
    .filter((item) => item.format);
  if (assets.length === 0) {
    throw new Error(
      `Electron Forge produced no ${target} release assets under ${makeDir}`,
    );
  }
  const missingAssets = definition.requiredAssets.filter(
    (kind) => !assets.some((asset) => asset.kind === kind),
  );
  if (missingAssets.length > 0) {
    throw new Error(
      `Electron Forge output for ${target} is incomplete; missing ${missingAssets.join(", ")}`,
    );
  }

  await ensureEmptyDirectory(outDir);
  const stagedArtifacts = [];
  const stagedNames = new Set();
  for (const asset of assets) {
    const name = normalizeArtifactName(target, asset.file);
    if (stagedNames.has(name)) {
      throw new Error(`duplicate staged desktop asset name: ${name}`);
    }
    stagedNames.add(name);
    const destination = join(outDir, name);
    await copyFile(asset.file, destination);
    const fileStat = await stat(destination);
    stagedArtifacts.push({
      name,
      format: asset.format,
      kind: asset.kind,
      size_bytes: fileStat.size,
      sha256: await sha256(destination),
    });
  }

  const metadata = {
    schema_version: metadataSchema,
    app_id: "run.zhongcao.contentcloud.desktop",
    channel,
    version,
    tag,
    target,
    platform: definition.platform,
    arch: definition.arch,
    generated_at: new Date().toISOString(),
    signing: definition.signed
      ? {
          required: true,
          status: signed === true ? "verified" : "unverified-preview",
        }
      : { required: false, status: "not-required-for-preview" },
    artifacts: stagedArtifacts.map((artifact) => ({
      ...artifact,
      download_url: repository
        ? `https://github.com/${repository}/releases/download/${tag}/${encodeURIComponent(artifact.name)}`
        : undefined,
    })),
  };
  const metadataPath = join(outDir, `${target}-latest.json`);
  await writeFile(
    metadataPath,
    `${JSON.stringify(metadata, null, 2)}\n`,
    "utf8",
  );
  await writeFile(
    join(outDir, `${target}-checksums.sha256`),
    `${stagedArtifacts.map((artifact) => `${artifact.sha256}  ${artifact.name}`).join("\n")}\n`,
    "utf8",
  );
  return metadata;
}

async function aggregateRelease({
  inputDir,
  outDir,
  channel,
  tag,
  repository,
  requireAllTargets = false,
}) {
  const inputFiles = await filesUnder(inputDir);
  const metadataFiles = inputFiles.filter((file) =>
    file.endsWith("-latest.json"),
  );
  if (metadataFiles.length === 0)
    throw new Error(`no staged target metadata under ${inputDir}`);
  const targets = {};
  const filesByName = new Map();
  for (const file of inputFiles) {
    const name = basename(file);
    if (filesByName.has(name)) {
      throw new Error(`duplicate staged release filename: ${name}`);
    }
    filesByName.set(name, file);
  }
  for (const file of metadataFiles) {
    const metadata = JSON.parse(await readFile(file, "utf8"));
    if (metadata.channel !== channel || metadata.tag !== tag) {
      throw new Error(
        `staged metadata identity mismatch: ${relative(inputDir, file)}`,
      );
    }
    const definition = targetDefinitions[metadata.target];
    if (!definition || targets[metadata.target]) {
      throw new Error(`invalid or duplicate staged target: ${metadata.target}`);
    }
    if (
      metadata.schema_version !== metadataSchema ||
      metadata.app_id !== "run.zhongcao.contentcloud.desktop" ||
      metadata.platform !== definition.platform ||
      metadata.arch !== definition.arch
    ) {
      throw new Error(`invalid staged metadata contract: ${metadata.target}`);
    }
    if (
      definition.signed &&
      (metadata.signing?.required !== true ||
        metadata.signing?.status !== "verified")
    ) {
      throw new Error(`staged target is not signed: ${metadata.target}`);
    }
    const artifacts = metadata.artifacts ?? [];
    const missingAssets = definition.requiredAssets.filter(
      (kind) => !artifacts.some((artifact) => artifact.kind === kind),
    );
    if (missingAssets.length > 0) {
      throw new Error(
        `staged target ${metadata.target} is incomplete; missing ${missingAssets.join(", ")}`,
      );
    }
    for (const artifact of artifacts) {
      const artifactPath = filesByName.get(artifact.name);
      if (!artifactPath) {
        throw new Error(`staged artifact is missing: ${artifact.name}`);
      }
      const fileStat = await stat(artifactPath);
      if (
        fileStat.size !== artifact.size_bytes ||
        (await sha256(artifactPath)) !== artifact.sha256
      ) {
        throw new Error(`staged artifact integrity mismatch: ${artifact.name}`);
      }
    }
    if (repository) {
      for (const artifact of metadata.artifacts) {
        artifact.download_url = `https://github.com/${repository}/releases/download/${tag}/${encodeURIComponent(artifact.name)}`;
      }
    }
    targets[metadata.target] = metadata;
  }
  const versions = new Set(
    Object.values(targets).map((metadata) => metadata.version),
  );
  if (versions.size !== 1) {
    throw new Error("staged target versions must be identical");
  }
  if (requireAllTargets) {
    const missingTargets = Object.keys(targetDefinitions).filter(
      (target) => !targets[target],
    );
    if (missingTargets.length > 0) {
      throw new Error(
        `desktop release is missing targets: ${missingTargets.join(", ")}`,
      );
    }
  }
  await ensureEmptyDirectory(outDir);
  const index = {
    schema_version: indexSchema,
    app_id: "run.zhongcao.contentcloud.desktop",
    channel,
    version: Object.values(targets)[0].version,
    tag,
    generated_at: new Date().toISOString(),
    targets,
  };
  const indexPath = join(outDir, `desktop-${channel}-latest.json`);
  await writeFile(indexPath, `${JSON.stringify(index, null, 2)}\n`, "utf8");

  const releaseFiles = await filesUnder(inputDir);
  const checksumLines = [];
  for (const file of releaseFiles) {
    const fileStat = await stat(file);
    if (!fileStat.isFile()) continue;
    checksumLines.push(
      `${await sha256(file)}  ${relative(inputDir, file).replaceAll("\\", "/")}`,
    );
  }
  checksumLines.push(`${await sha256(indexPath)}  ${basename(indexPath)}`);
  await writeFile(
    join(outDir, "desktop-checksums.txt"),
    `${checksumLines.sort().join("\n")}\n`,
    "utf8",
  );
  return index;
}

async function main() {
  const [command = "stage", ...rest] = process.argv.slice(2);
  const args = parseArgs(rest);
  if (command === "aggregate") {
    await aggregateRelease({
      inputDir: resolve(required(args, "input-dir")),
      outDir: resolve(required(args, "out-dir")),
      channel: required(args, "channel"),
      tag: required(args, "tag"),
      repository: args.repository?.trim(),
      requireAllTargets: args["require-all-targets"] === true,
    });
    return;
  }
  if (command !== "stage") throw new Error(`unknown command: ${command}`);
  const target = required(args, "target");
  const metadata = await stageTarget({
    forgeDir: resolve(required(args, "forge-dir")),
    outDir: resolve(required(args, "out-dir")),
    target,
    version: required(args, "version"),
    channel: required(args, "channel"),
    tag: required(args, "tag"),
    repository: args.repository?.trim(),
    signed: args.signed === true,
    preview: args.preview === true,
  });
  process.stdout.write(`${JSON.stringify(metadata, null, 2)}\n`);
}

const entrypoint = process.argv[1];
if (entrypoint && import.meta.url === pathToFileURL(resolve(entrypoint)).href) {
  try {
    await main();
  } catch (error) {
    console.error(`desktop release staging failed: ${error.message}`);
    process.exitCode = 1;
  }
}

export { aggregateRelease, stageTarget, targetDefinitions };
