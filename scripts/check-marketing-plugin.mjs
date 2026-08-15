#!/usr/bin/env node

import { createHash } from 'node:crypto';
import { readdir, readFile, stat } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import process from 'node:process';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const pluginName = 'contentcloud-marketing';
const pluginVersion = '0.1.0';
const pluginRoot = resolve(root, 'plugins', pluginName);
const expectedSkills = [
  'contentcloud-marketing-client-agent-delivery',
  'contentcloud-marketing-client-knowledge-pack',
  'contentcloud-marketing-content-compile',
  'contentcloud-marketing-intent-content',
  'contentcloud-marketing-knowledge-ingest',
  'contentcloud-marketing-knowledge-lint',
  'contentcloud-marketing-knowledge-pipeline',
  'contentcloud-marketing-knowledge-query',
];
const failures = [];
const fail = (message) => failures.push(message);
const readJSON = async (relative) => JSON.parse(await readFile(resolve(root, relative), 'utf8'));

const manifest = await readJSON(`plugins/${pluginName}/plugin.json`);
const claims = await readJSON(`plugins/${pluginName}/run.zhongcao.contentcloud/claims.json`);
const registry = await readJSON('.agents/plugins/registry.draft.json');
const registryEntry = registry.entries?.find((entry) => entry?.id === pluginName);
const skillRoot = resolve(pluginRoot, 'skills');
const actualSkills = (await readdir(skillRoot, { withFileTypes: true }))
  .filter((entry) => entry.isDirectory())
  .map((entry) => entry.name)
  .sort();

if (process.argv.includes('--digest-only')) {
  process.stdout.write(`${await directoryDigest(pluginRoot)}\n`);
  process.exit(0);
}

if (manifest.name !== pluginName || manifest.version !== pluginVersion) fail('manifest identity must match contentcloud-marketing@0.1.0');
if (manifest.extensions?.['run.zhongcao.contentcloud']?.claims !== './run.zhongcao.contentcloud/claims.json') fail('manifest must expose ContentCloud claims');
if (actualSkills.join('\n') !== expectedSkills.join('\n')) fail(`skills must be exactly ${JSON.stringify(expectedSkills)}`);
if (await exists(resolve(pluginRoot, 'mcp.json'))) fail('marketing Skill Pack must not ship a second MCP server');
if (claims.kind !== 'skill_pack' || claims.plugin_id !== pluginName || claims.plugin_version !== pluginVersion) fail('claims must identify the marketing Skill Pack');
if (claims.hosts?.some((host) => !host.required?.includes('skills') || !host.required?.includes('new_session_required'))) fail('every supported host must require Skills and a new session');
if (claims.hosts?.some((host) => host.required?.includes('mcp_stdio'))) fail('marketing Skill Pack must reuse Core stdio MCP instead of declaring one');

const capabilityIDs = new Set(claims.requested_capabilities?.map((capability) => capability.id));
if (capabilityIDs.size !== 2 || !capabilityIDs.has('contentcloud.marketing.knowledge-governance') || !capabilityIDs.has('contentcloud.marketing.content-orchestration')) {
  fail('claims must declare the two marketing orchestration capabilities');
}
if (registryEntry?.kind !== 'skill_pack' || registryEntry?.version !== pluginVersion) fail('registry must contain the marketing Skill Pack as version 0.1.0');
if (!registryEntry?.compatible_profiles?.includes('contentcloud.video-production')) fail('registry must allow composition with the video-production environment');
if (registryEntry?.signature?.status !== 'pending' || registryEntry?.evaluation?.status !== 'pending' || registryEntry?.lifecycle !== 'draft') fail('unreviewed marketing release must remain pending and draft');
if (registryEntry?.digest !== `sha256:${await directoryDigest(pluginRoot)}`) fail('registry marketing digest does not match package contents');

const forbidden = [
  '../service',
  'repoRoot',
  'run-context.mjs',
  'Node 服务',
  'Ruby YAML',
];
for (const skill of expectedSkills) {
  const relative = `plugins/${pluginName}/skills/${skill}/SKILL.md`;
  const body = await readFile(resolve(root, relative), 'utf8');
  if (!/^name:\s*\S+/m.test(body)) fail(`${relative} must declare a name`);
  if (!/^description:.*[\u3400-\u9fff]/m.test(body)) fail(`${relative} description must be Chinese`);
  for (const token of ['workspace_context', 'local_run']) {
    if (!body.includes(token)) fail(`${relative} must use ${token}`);
  }
  for (const token of forbidden) if (body.includes(token)) fail(`${relative} contains forbidden customer/runtime reference ${token}`);
  if (/(?:\/Users\/|\/home\/|[A-Za-z]:\\\\)/.test(body)) fail(`${relative} contains an absolute filesystem path`);
}
const compile = await readFile(resolve(pluginRoot, 'skills/contentcloud-marketing-content-compile/SKILL.md'), 'utf8');
for (const token of ['contentcloud-video-production', '$contentcloud-marketing-video-script', 'contentcloud-wechat-article', '$contentcloud-article-planning', 'content_batch_lint', 'publish_preflight']) {
  if (!compile.includes(token)) fail(`content compiler must orchestrate ${token}`);
}
const pipeline = await readFile(resolve(pluginRoot, 'skills/contentcloud-marketing-knowledge-pipeline/SKILL.md'), 'utf8');
for (const token of ['intent:ingest', 'intent:query', 'intent:content', 'local_run_record', 'local_run_check', 'local_run_advance', 'local_run_fail', 'local_run_resume']) {
  if (!pipeline.includes(token)) fail(`marketing pipeline is missing ${token}`);
}
const delivery = await readFile(resolve(pluginRoot, 'skills/contentcloud-marketing-client-agent-delivery/SKILL.md'), 'utf8');
for (const token of ['client-knowledge-pack', 'intent-content', '$contentcloud-marketing-video-script', '$contentcloud-article-planning', 'resume', 'publish_apply']) {
  if (!delivery.includes(token)) fail(`client delivery orchestrator is missing ${token}`);
}
const boundary = await readFile(resolve(pluginRoot, 'references/workspace-boundary.md'), 'utf8');
for (const token of ['Workspace 数据', 'Plugin 能力', 'Core 能力', '第二套 RunContext']) {
  if (!boundary.includes(token)) fail(`workspace boundary is missing ${token}`);
}

const report = {
  ok: failures.length === 0,
  plugin: `${pluginName}@${pluginVersion}`,
  digest: `sha256:${await directoryDigest(pluginRoot)}`,
  skills: actualSkills,
  mcp_servers: 0,
  capabilities: [...capabilityIDs].sort(),
  registry: registryEntry ? { lifecycle: registryEntry.lifecycle, signature: registryEntry.signature?.status, evaluation: registryEntry.evaluation?.status } : null,
  failures,
};
process.stdout.write(`${JSON.stringify(report, null, 2)}\n`);
if (failures.length > 0) process.exit(1);

async function exists(path) {
  try {
    await stat(path);
    return true;
  } catch {
    return false;
  }
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
    else throw new Error(`unsupported package file type: ${path}`);
  }
  return result;
}
