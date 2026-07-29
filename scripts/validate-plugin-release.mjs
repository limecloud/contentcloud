#!/usr/bin/env node

import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { readdir, readFile, stat } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import process from 'node:process';
import { publicKeyFingerprint, verifyRegistryEntrySignature } from './lib/plugin-release.mjs';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const pluginName = 'contentcloud-video-production';
const pluginRelativePath = `plugins/${pluginName}`;
const pluginRoot = resolve(root, pluginRelativePath);
const digest = await directoryDigest(pluginRoot);

if (process.argv.includes('--digest-only')) {
  process.stdout.write(`${digest}\n`);
  process.exit(0);
}

const tagged = process.argv.includes('--tagged');
const errors = [];
const warnings = [];
const check = (condition, message) => {
  if (!condition) errors.push(message);
};

const workspacePackage = await readJSON('package.json');
const cliPackage = await readJSON('packages/contentcloud/package.json');
const webPackage = await readJSON('web/package.json');
const marketplace = await readJSON('.agents/plugins/marketplace.json');
const registry = await readJSON('.agents/plugins/registry.json');
const trustStore = await readJSON('.agents/plugins/trusted-keys.json');
const environmentProfile = await readJSON('deploy/systemd/environment-profile.json');
const plugin = await readJSON(`${pluginRelativePath}/.codex-plugin/plugin.json`);
const mcp = await readJSON(`${pluginRelativePath}/.mcp.json`);
const versionFile = (await readText('VERSION')).trim();
const goSource = await readText('internal/cli/root.go');
const codexGuideSource = await readText('internal/httpapi/codex.go');
const bootstrapSource = await readText('internal/httpapi/bootstrap.md');
const webSource = await readText('web/src/connectBootstrap.ts');
const codexHandoffSource = await readText('web/src/codexHandoff.ts');
const systemdEnvironmentSource = await readText('deploy/systemd/contentcloud.env.example');
const license = await readText('LICENSE');

const goVersion = exactMatch(goSource, /const\s+Version\s*=\s*"([^"]+)"/, 'internal/cli Version');
const codexGuideVersion = exactMatch(codexGuideSource, /codexGuideVersion\s*=\s*"([^"]+)"/, 'internal/httpapi /codex guide version');
const webCLIVersion = exactMatch(webSource, /@limecloud\/contentcloud@([^'\s]+)'/, 'web CONTENTCLOUD_CLI version');
const codexHandoffVersion = exactMatch(codexHandoffSource, /plugin_version\s*!==\s*'([^']+)'/, 'web Codex handoff Plugin version');
const capabilityReleaseVersion = exactMatch(systemdEnvironmentSource, /^CONTENTCLOUD_CAPABILITY_RELEASE_VERSION=([^\s]+)$/m, 'systemd capability release version');
const bootstrapVersions = [...bootstrapSource.matchAll(/@limecloud\/contentcloud@([^\s`]+)/g)].map(match => match[1]);
const mcpServer = mcp.mcpServers?.['contentcloud-local'];
const mcpPackage = Array.isArray(mcpServer?.args)
  ? mcpServer.args.find(value => typeof value === 'string' && value.startsWith('@limecloud/contentcloud@'))
  : undefined;
const mcpVersion = mcpPackage?.slice('@limecloud/contentcloud@'.length);

const versions = new Map([
  ['VERSION', versionFile],
  ['package.json', workspacePackage.version],
  ['packages/contentcloud/package.json', cliPackage.version],
  ['web/package.json', webPackage.version],
  ['plugin.json', plugin.version],
  ['deploy/systemd/environment-profile.json', environmentProfile.plugins?.find(candidate => candidate?.id === pluginName)?.version],
  ['deploy/systemd/contentcloud.env.example', capabilityReleaseVersion],
  ['internal/cli/root.go', goVersion],
  ['internal/httpapi/codex.go', codexGuideVersion],
  ['web/src/connectBootstrap.ts', webCLIVersion],
  ['web/src/codexHandoff.ts', codexHandoffVersion],
  ['plugin .mcp.json', mcpVersion],
]);
for (const [source, value] of versions) {
  check(typeof value === 'string' && value === versionFile, `${source} version ${JSON.stringify(value)} does not match VERSION ${versionFile}`);
}
check(/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(versionFile), `VERSION is not a supported semantic version: ${versionFile}`);
check(cliPackage.contentcloudReleaseTag === `v${versionFile}`, 'npm contentcloudReleaseTag must equal v<VERSION>');
check(bootstrapVersions.length > 0 && bootstrapVersions.every(value => value === versionFile), `internal/httpapi/bootstrap.md contains versions ${JSON.stringify([...new Set(bootstrapVersions)])}, expected ${versionFile}`);

check(plugin.name === pluginName, 'plugin manifest name must match its directory');
check(plugin.license === 'Apache-2.0', 'plugin manifest license must be Apache-2.0');
check(cliPackage.license === plugin.license, 'npm and plugin licenses must match');
check(license.includes('Apache License') && license.includes('Version 2.0'), 'root LICENSE must contain Apache License 2.0');
check(plugin.skills === './skills/', 'plugin manifest must expose bundled skills from ./skills/');
check(plugin.mcpServers === './.mcp.json', 'plugin manifest must reference ./.mcp.json');
check(mcpServer?.command === 'npx', 'contentcloud-local MCP command must be npx');
check(JSON.stringify(mcpServer?.args?.slice(-2)) === JSON.stringify(['mcp', 'serve']), 'contentcloud-local MCP must invoke mcp serve');

check(marketplace.name === 'contentcloud', 'repo marketplace name must be contentcloud');
const marketplaceEntries = Array.isArray(marketplace.plugins) ? marketplace.plugins : [];
const marketplaceEntry = marketplaceEntries.find(entry => entry?.name === pluginName);
check(Boolean(marketplaceEntry), `marketplace is missing ${pluginName}`);
check(marketplaceEntry?.source?.source === 'local', 'marketplace plugin source must be local to the immutable Git ref');
check(marketplaceEntry?.source?.path === `./${pluginRelativePath}`, 'marketplace plugin path must point to the canonical plugin directory');
check(marketplaceEntry?.policy?.installation === 'AVAILABLE', 'marketplace installation policy must be AVAILABLE');
check(['ON_INSTALL', 'ON_USE'].includes(marketplaceEntry?.policy?.authentication), 'marketplace authentication policy is invalid');
check(typeof marketplaceEntry?.category === 'string' && marketplaceEntry.category !== '', 'marketplace category is required');

check(registry.schema_version === '1.0', 'registry schema_version must be 1.0');
check(registry.$schema === '../../contracts/marketplace-registry-1.0.schema.json', 'registry must reference the checked-in 1.0 schema');
check(trustStore.schema_version === '1.0', 'trusted key store schema_version must be 1.0');
check(trustStore.$schema === '../../contracts/plugin-trusted-keys-1.0.schema.json', 'trusted key store must reference the checked-in 1.0 schema');
const trustedKeys = Array.isArray(trustStore.keys) ? trustStore.keys : [];
check(Array.isArray(trustStore.keys), 'trusted key store keys must be an array');
const trustedKeyIDs = new Set();
for (const key of trustedKeys) {
  check(typeof key?.key_id === 'string' && /^[a-z0-9]+(?:[._-][a-z0-9]+)*$/.test(key.key_id), 'trusted key_id is invalid');
  check(!trustedKeyIDs.has(key?.key_id), `trusted key_id ${JSON.stringify(key?.key_id)} is duplicated`);
  trustedKeyIDs.add(key?.key_id);
  check(key?.algorithm === 'ed25519', `trusted key ${JSON.stringify(key?.key_id)} algorithm must be ed25519`);
  check(['active', 'revoked'].includes(key?.status), `trusted key ${JSON.stringify(key?.key_id)} status is invalid`);
  try {
    publicKeyFingerprint(key?.public_key);
  } catch (error) {
    errors.push(`trusted key ${JSON.stringify(key?.key_id)} is invalid: ${error.message}`);
  }
}
const registryEntries = Array.isArray(registry.entries) ? registry.entries : [];
const registryEntry = registryEntries.find(entry => entry?.id === pluginName);
check(Boolean(registryEntry), `governance registry is missing ${pluginName}`);
check(registryEntry?.kind === 'scene_plugin', 'registry kind must be scene_plugin');
check(registryEntry?.version === versionFile, 'registry plugin version must match VERSION');
check(registryEntry?.source?.repository === 'https://github.com/limecloud/contentcloud', 'registry repository is not pinned to the ContentCloud source');
check(registryEntry?.source?.ref === `v${versionFile}`, 'registry source ref must match VERSION');
check(registryEntry?.license === plugin.license, 'registry license must match plugin manifest');
check(registryEntry?.digest === `sha256:${digest}`, 'registry digest does not match canonical plugin contents');
check(Array.isArray(registryEntry?.compatible_profiles) && registryEntry.compatible_profiles.includes('contentcloud.video-production'), 'registry must allow the video-production profile');
check(Array.isArray(registryEntry?.permissions) && registryEntry.permissions.length > 0, 'registry permissions must be explicit');
check(['free', 'included', 'metered', 'external'].includes(registryEntry?.cost?.model), 'registry cost model must be explicit');
check(typeof registryEntry?.cost?.notice === 'string' && registryEntry.cost.notice.trim() !== '', 'registry cost notice must be explicit');
if (registryEntry?.cost?.model === 'metered') {
  check(/^[A-Z]{3}$/.test(registryEntry.cost.currency ?? ''), 'metered registry cost requires an ISO currency');
  check(typeof registryEntry.cost.unit === 'string' && registryEntry.cost.unit.trim() !== '', 'metered registry cost requires a unit');
  check(/^(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$/.test(registryEntry.cost.unit_price ?? ''), 'metered registry cost requires a decimal unit_price');
}
check(['draft', 'security_review', 'evaluated', 'published', 'deprecated', 'revoked'].includes(registryEntry?.lifecycle), 'registry lifecycle is invalid');
check(['active', 'revoked'].includes(registryEntry?.revocation?.status), 'registry revocation status is invalid');
check(['pending', 'verified'].includes(registryEntry?.signature?.status), 'registry signature status is invalid');
check(['pending', 'passed', 'failed'].includes(registryEntry?.evaluation?.status), 'registry evaluation status is invalid');
if (registryEntry?.revocation?.status === 'revoked') {
  check(registryEntry.lifecycle === 'revoked', 'revoked registry entry lifecycle must be revoked');
  check(['advisory', 'high'].includes(registryEntry.revocation.severity), 'revoked registry entry severity must be advisory or high');
  check(typeof registryEntry.revocation.reason === 'string' && registryEntry.revocation.reason.trim() !== '', 'revoked registry entry requires a reason');
} else {
  check(registryEntry?.lifecycle !== 'revoked', 'revoked lifecycle requires revoked registry status');
}

const evaluation = registryEntry?.evaluation;
if (evaluation?.status === 'passed') {
  check(typeof evaluation.report === 'string' && evaluation.report.startsWith('.agents/plugins/evaluations/'), 'passed evaluation must reference a checked-in report');
  check(/^sha256:[a-f0-9]{64}$/.test(evaluation.digest ?? ''), 'passed evaluation must declare a SHA-256 report digest');
  check(Array.isArray(evaluation.evidence) && evaluation.evidence.length > 0, 'passed evaluation must declare scenario evidence');
  if (typeof evaluation.report === 'string') {
    const evaluationBody = await readText(evaluation.report).catch(error => {
      errors.push(`cannot read evaluation report ${evaluation.report}: ${error.message}`);
      return undefined;
    });
    if (evaluationBody !== undefined) {
      const reportDigest = `sha256:${createHash('sha256').update(evaluationBody).digest('hex')}`;
      check(evaluation.digest === reportDigest, 'evaluation report digest does not match registry');
      let evaluationReport;
      try {
        evaluationReport = JSON.parse(evaluationBody);
      } catch (error) {
        errors.push(`${evaluation.report} is not valid JSON: ${error.message}`);
      }
      if (evaluationReport) {
        check(evaluationReport.schema_version === '1.0', 'evaluation report schema_version must be 1.0');
        check(evaluationReport.scope === 'deterministic_release_contract', 'evaluation report scope is invalid');
        check(evaluationReport.status === 'passed', 'evaluation report status must be passed');
        check(evaluationReport.plugin?.id === pluginName, 'evaluation report Plugin ID does not match');
        check(evaluationReport.plugin?.version === versionFile, 'evaluation report Plugin version does not match');
        check(evaluationReport.plugin?.digest === `sha256:${digest}`, 'evaluation report Plugin digest does not match');
        const scenarios = Array.isArray(evaluationReport.scenarios) ? evaluationReport.scenarios : [];
        check(scenarios.length > 0 && scenarios.every(scenario => scenario?.status === 'passed'), 'all evaluation report scenarios must be passed');
        const scenarioIDs = scenarios.map(scenario => scenario?.id);
        check(evaluation.evidence.every(id => scenarioIDs.includes(id)), 'registry evaluation evidence must reference report scenarios');
      }
    }
  }
}

const outputSchemas = Array.isArray(registryEntry?.output_schemas) ? registryEntry.output_schemas : [];
check(outputSchemas.length > 0, 'registry output_schemas must not be empty');
for (const schemaPath of outputSchemas) {
  const schema = await readJSON(schemaPath);
  check(schema?.$schema === 'https://json-schema.org/draft/2020-12/schema', `${schemaPath} must use JSON Schema draft 2020-12`);
  check(typeof schema?.title === 'string' && schema.title !== '', `${schemaPath} must have a title`);
  check(schema?.type === 'object' || schema?.type === 'array', `${schemaPath} must declare an object or array root type`);
}

const controlPlaneSchemas = [
  'contracts/creative-environment-manifest-1.0.schema.json',
  'contracts/creative-environment-profile-1.0.schema.json',
  'contracts/environment-lock-1.0.schema.json',
  'contracts/local-execution-plan-1.0.schema.json',
  'contracts/environment-preparation-plan-1.0.schema.json',
  'contracts/creative-execution-bundle-1.0.schema.json',
  'contracts/environment-trusted-keys-1.0.schema.json',
];
for (const schemaPath of controlPlaneSchemas) {
  const schema = await readJSON(schemaPath);
  check(schema?.$schema === 'https://json-schema.org/draft/2020-12/schema', `${schemaPath} must use JSON Schema draft 2020-12`);
  check(typeof schema?.$id === 'string' && schema.$id !== '', `${schemaPath} must have an $id`);
  check(typeof schema?.title === 'string' && schema.title !== '', `${schemaPath} must have a title`);
  check(schema?.type === 'object', `${schemaPath} must declare an object root type`);
}

const pluginFiles = await filesUnder(pluginRoot);
check(!pluginFiles.some(path => path.endsWith('.DS_Store')), 'plugin contains .DS_Store metadata');
const executableScripts = [];
for (const path of pluginFiles) {
  const body = await readFile(resolve(pluginRoot, path), 'utf8');
  check(!body.includes('[TODO:'), `${pluginRelativePath}/${path} contains a TODO placeholder`);
  const mode = (await stat(resolve(pluginRoot, path))).mode & 0o777;
  if ((mode & 0o111) !== 0) {
    check(path === 'scripts' || path.startsWith('scripts/'), `${pluginRelativePath}/${path} is executable outside scripts/`);
    executableScripts.push(path);
  }
}
const skillDirectories = await directoryNames(resolve(pluginRoot, 'skills'));
const expectedSkillDirectories = [
  'contentcloud-douyin-audience-strategy',
  'contentcloud-knowledge-extraction',
  'contentcloud-marketing-video-script',
  'contentcloud-seedance-export',
  'contentcloud-storyboard-production',
  'contentcloud-workspace',
];
check(
  JSON.stringify(skillDirectories) === JSON.stringify(expectedSkillDirectories),
  `bundled skills ${JSON.stringify(skillDirectories)} do not match expected ${JSON.stringify(expectedSkillDirectories)}`,
);
for (const skill of skillDirectories) {
  const skillMarkdown = await readText(`${pluginRelativePath}/skills/${skill}/SKILL.md`);
  const declaredName = exactMatch(skillMarkdown, /^name:\s*["']?([^\n"']+)["']?\s*$/m, `${skill} skill name`);
  check(declaredName === skill, `${skill} SKILL.md declares name ${JSON.stringify(declaredName)}`);
  await stat(resolve(pluginRoot, 'skills', skill, 'agents', 'openai.yaml')).catch(() => {
    errors.push(`${skill} is missing agents/openai.yaml`);
  });
}

if (registryEntry?.signature?.status !== 'verified') {
  warnings.push('registry signature is pending; tagged release validation will fail');
} else {
  try {
    verifyRegistryEntrySignature(registryEntry, trustStore);
  } catch (error) {
    errors.push(`registry signature is invalid: ${error.message}`);
  }
}
if (registryEntry?.evaluation?.status !== 'passed') {
  warnings.push('scene evaluation is pending; tagged release validation will fail');
}

const sourceRef = registryEntry?.source?.ref;
if (sourceRef && gitRefExists(sourceRef)) {
  const manifestAtRef = `${sourceRef}:${pluginRelativePath}/.codex-plugin/plugin.json`;
  if (!gitObjectExists(manifestAtRef)) {
    warnings.push(`${sourceRef} exists but does not contain the plugin manifest; choose a new immutable version`);
  }
}

if (tagged) {
  check(registryEntry?.signature?.status === 'verified', 'tagged release requires a verified registry signature');
  check(registryEntry?.signature?.algorithm === 'ed25519', 'tagged release requires an Ed25519 signature');
  check(typeof registryEntry?.signature?.key_id === 'string' && registryEntry.signature.key_id !== '', 'tagged release requires a trusted signature key_id');
  check(typeof registryEntry?.signature?.value === 'string' && registryEntry.signature.value !== '', 'tagged release requires a signature value');
  check(registryEntry?.evaluation?.status === 'passed', 'tagged release requires a passed scene evaluation');
  check(registryEntry?.lifecycle === 'evaluated' || registryEntry?.lifecycle === 'published', 'tagged release requires evaluated or published lifecycle');
  check(Boolean(sourceRef) && gitRefExists(sourceRef), 'tagged release source ref does not exist');
  check(Boolean(sourceRef) && gitObjectExists(`${sourceRef}:${pluginRelativePath}/.codex-plugin/plugin.json`), 'tagged release source ref does not contain the plugin');
}

const report = {
  ok: errors.length === 0,
  mode: tagged ? 'tagged' : 'source',
  version: versionFile,
  plugin: `${pluginName}@${plugin.version}`,
  plugin_digest: `sha256:${digest}`,
  marketplace_entries: marketplaceEntries.length,
  registry_entries: registryEntries.length,
  bundled_skills: skillDirectories,
  executable_scripts: executableScripts,
  output_schemas: outputSchemas,
  warnings,
  errors,
};
process.stdout.write(`${JSON.stringify(report, null, 2)}\n`);
if (errors.length > 0) process.exit(1);

async function readText(path) {
  return readFile(resolve(root, path), 'utf8');
}

async function readJSON(path) {
  const body = await readText(path);
  try {
    return JSON.parse(body);
  } catch (error) {
    throw new Error(`${path} is not valid JSON: ${error.message}`);
  }
}

function exactMatch(value, pattern, label) {
  const matches = [...value.matchAll(new RegExp(pattern.source, pattern.flags.includes('g') ? pattern.flags : `${pattern.flags}g`))];
  check(matches.length === 1, `${label} must appear exactly once`);
  return matches[0]?.[1];
}

async function directoryDigest(directory) {
  const hash = createHash('sha256');
  for (const path of await filesUnder(directory)) {
    const executable = ((await stat(resolve(directory, path))).mode & 0o111) !== 0;
    hash.update(path);
    hash.update('\0');
    hash.update(executable ? 'executable' : 'regular');
    hash.update('\0');
    hash.update(await readFile(resolve(directory, path)));
    hash.update('\0');
  }
  return hash.digest('hex');
}

async function filesUnder(directory, prefix = '') {
  const result = [];
  const entries = await readdir(resolve(directory, prefix), { withFileTypes: true });
  for (const entry of entries.sort((left, right) => left.name < right.name ? -1 : left.name > right.name ? 1 : 0)) {
    const path = prefix ? `${prefix}/${entry.name}` : entry.name;
    if (entry.isDirectory()) result.push(...await filesUnder(directory, path));
    else if (entry.isFile()) result.push(path);
    else throw new Error(`unsupported plugin file type: ${path}`);
  }
  return result;
}

async function directoryNames(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  return entries.filter(entry => entry.isDirectory()).map(entry => entry.name).sort();
}

function gitRefExists(ref) {
  return gitObjectExists(`${ref}^{commit}`);
}

function gitObjectExists(object) {
  try {
    execFileSync('git', ['cat-file', '-e', object], { cwd: root, stdio: 'ignore' });
    return true;
  } catch {
    return false;
  }
}
