import { createHash } from 'node:crypto';
import { mkdir, readdir, readFile, stat, writeFile, copyFile } from 'node:fs/promises';
import { basename, dirname, extname, join, relative, resolve } from 'node:path';

const root = resolve(import.meta.dirname, '..');
const policyPath = resolve(root, 'apps/desktop/update-channels.json');
const metadataSchema = 'contentcloud.desktop-update-metadata/1.0';
const indexSchema = 'contentcloud.desktop-update-index/1.0';

const targetDefinitions = {
  'darwin-arm64': { platform: 'darwin', arch: 'arm64', formats: ['dmg', 'zip'], signed: true },
  'darwin-x64': { platform: 'darwin', arch: 'x64', formats: ['dmg', 'zip'], signed: true },
  'win32-x64': { platform: 'win32', arch: 'x64', formats: ['squirrel'], signed: true },
  'linux-x64': { platform: 'linux', arch: 'x64', formats: ['deb', 'rpm'], signed: false },
};

const formatForFile = (file, target) => {
  const extension = extname(file).toLowerCase().replace(/^\./, '');
  if (target === 'win32-x64') {
    if (extension === 'exe' || extension === 'nupkg' || basename(file).toUpperCase() === 'RELEASES') {
      return 'squirrel';
    }
  }
  if (target.startsWith('darwin-') && ['dmg', 'zip'].includes(extension)) return extension;
  if (target === 'linux-x64' && ['deb', 'rpm'].includes(extension)) return extension;
  return undefined;
};

const normalizeArtifactName = (target, file) =>
  `${target}-${basename(file).replace(/[^A-Za-z0-9._-]+/g, '-')}`;

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
  return createHash('sha256').update(content).digest('hex');
}

function parseArgs(argv) {
  const args = {};
  for (let index = 0; index < argv.length; index += 1) {
    const token = argv[index];
    if (!token.startsWith('--')) continue;
    const key = token.slice(2);
    const next = argv[index + 1];
    if (!next || next.startsWith('--')) {
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
  if (typeof value !== 'string' || value.trim() === '') {
    throw new Error(`missing --${key}`);
  }
  return value.trim();
}

function validateReleaseIdentity({ version, channel, tag }) {
  if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(version)) {
    throw new Error(`invalid release version: ${version}`);
  }
  if (!['stable', 'beta'].includes(channel)) {
    throw new Error(`invalid release channel: ${channel}`);
  }
  if (!tag || !tag.endsWith(version)) {
    throw new Error(`release tag must end with version ${version}: ${tag}`);
  }
}

async function stageTarget({ forgeDir, outDir, target, version, channel, tag, repository, signed }) {
  const definition = targetDefinitions[target];
  if (!definition) throw new Error(`unsupported desktop target: ${target}`);
  validateReleaseIdentity({ version, channel, tag });
  if (definition.signed && signed !== true) {
    throw new Error(`${target} requires verified signing before it can enter ${channel}`);
  }

  const makeDir = join(forgeDir, 'make');
  const sourceFiles = await filesUnder(makeDir);
  const assets = sourceFiles
    .map((file) => ({ file, format: formatForFile(file, target) }))
    .filter((item) => item.format);
  if (assets.length === 0) {
    throw new Error(`Electron Forge produced no ${target} release assets under ${makeDir}`);
  }

  await ensureEmptyDirectory(outDir);
  const stagedArtifacts = [];
  for (const asset of assets) {
    const name = normalizeArtifactName(target, asset.file);
    const destination = join(outDir, name);
    await copyFile(asset.file, destination);
    const fileStat = await stat(destination);
    stagedArtifacts.push({
      name,
      format: asset.format,
      size_bytes: fileStat.size,
      sha256: await sha256(destination),
    });
  }

  const metadata = {
    schema_version: metadataSchema,
    app_id: 'run.zhongcao.contentcloud.desktop',
    channel,
    version,
    tag,
    target,
    platform: definition.platform,
    arch: definition.arch,
    generated_at: new Date().toISOString(),
    signing: definition.signed
      ? { required: true, status: 'verified' }
      : { required: false, status: 'not-required-for-preview' },
    artifacts: stagedArtifacts.map((artifact) => ({
      ...artifact,
      download_url: repository
        ? `https://github.com/${repository}/releases/download/${tag}/${encodeURIComponent(artifact.name)}`
        : undefined,
    })),
  };
  const metadataPath = join(outDir, `${target}-latest.json`);
  await writeFile(metadataPath, `${JSON.stringify(metadata, null, 2)}\n`, 'utf8');
  await writeFile(
    join(outDir, `${target}-checksums.sha256`),
    `${stagedArtifacts.map((artifact) => `${artifact.sha256}  ${artifact.name}`).join('\n')}\n`,
    'utf8',
  );
  return metadata;
}

async function aggregateRelease({ inputDir, outDir, channel, tag, repository }) {
  const metadataFiles = (await filesUnder(inputDir)).filter((file) => file.endsWith('-latest.json'));
  if (metadataFiles.length === 0) throw new Error(`no staged target metadata under ${inputDir}`);
  const targets = {};
  for (const file of metadataFiles) {
    const metadata = JSON.parse(await readFile(file, 'utf8'));
    if (metadata.channel !== channel || metadata.tag !== tag) {
      throw new Error(`staged metadata identity mismatch: ${relative(inputDir, file)}`);
    }
    if (repository) {
      for (const artifact of metadata.artifacts) {
        artifact.download_url = `https://github.com/${repository}/releases/download/${tag}/${encodeURIComponent(artifact.name)}`;
      }
    }
    targets[metadata.target] = metadata;
  }
  await ensureEmptyDirectory(outDir);
  const index = {
    schema_version: indexSchema,
    app_id: 'run.zhongcao.contentcloud.desktop',
    channel,
    version: Object.values(targets)[0].version,
    tag,
    generated_at: new Date().toISOString(),
    targets,
  };
  const indexPath = join(outDir, `desktop-${channel}-latest.json`);
  await writeFile(indexPath, `${JSON.stringify(index, null, 2)}\n`, 'utf8');

  const releaseFiles = (await filesUnder(inputDir)).filter(
    (file) => !file.endsWith('-checksums.sha256') && !file.endsWith('-latest.json'),
  );
  const checksumLines = [];
  for (const file of releaseFiles) {
    const fileStat = await stat(file);
    if (!fileStat.isFile()) continue;
    checksumLines.push(`${await sha256(file)}  ${relative(inputDir, file).replaceAll('\\', '/')}`);
  }
  checksumLines.push(`${await sha256(indexPath)}  ${basename(indexPath)}`);
  await writeFile(join(outDir, 'checksums.txt'), `${checksumLines.sort().join('\n')}\n`, 'utf8');
  return index;
}

async function main() {
  const [command = 'stage', ...rest] = process.argv.slice(2);
  const args = parseArgs(rest);
  if (command === 'aggregate') {
    await aggregateRelease({
      inputDir: resolve(required(args, 'input-dir')),
      outDir: resolve(required(args, 'out-dir')),
      channel: required(args, 'channel'),
      tag: required(args, 'tag'),
      repository: args.repository?.trim(),
    });
    return;
  }
  if (command !== 'stage') throw new Error(`unknown command: ${command}`);
  const target = required(args, 'target');
  const metadata = await stageTarget({
    forgeDir: resolve(required(args, 'forge-dir')),
    outDir: resolve(required(args, 'out-dir')),
    target,
    version: required(args, 'version'),
    channel: required(args, 'channel'),
    tag: required(args, 'tag'),
    repository: args.repository?.trim(),
    signed: args.signed === true,
  });
  process.stdout.write(`${JSON.stringify(metadata, null, 2)}\n`);
}

if (import.meta.url === `file://${process.argv[1]}`) {
  try {
    await main();
  } catch (error) {
    console.error(`desktop release staging failed: ${error.message}`);
    process.exitCode = 1;
  }
}

export { aggregateRelease, stageTarget, targetDefinitions };
