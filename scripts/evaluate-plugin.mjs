#!/usr/bin/env node

import { execFileSync, spawnSync } from 'node:child_process';
import { readFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import process from 'node:process';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const plugin = JSON.parse(await readFile(resolve(root, 'plugins/contentcloud-video-production/plugin.json'), 'utf8'));
const version = plugin.version;
const reportPath = resolve(root, `.agents/plugins/evaluations/contentcloud-video-production-${version}.json`);
const report = JSON.parse(await readFile(reportPath, 'utf8'));
const pluginDigest = execFileSync(process.execPath, ['scripts/validate-plugin-release.mjs', '--digest-only'], { cwd: root, encoding: 'utf8' }).trim();
const failures = [];
const internalPath = (...parts) => `./${['internal', ...parts].join('/')}`;
const packagePaths = new Map([
  [internalPath('agentadapter'), internalPath('integration', 'agent')],
  [internalPath('app'), internalPath('application')],
  [internalPath('automationworkspace'), internalPath('local', 'automation')],
  [internalPath('bootstrapcheck'), internalPath('bootstrap', 'check')],
  [internalPath('capabilitycatalog'), internalPath('catalog', 'capability')],
  [internalPath('cli'), internalPath('transport', 'cli')],
  [internalPath('environment'), internalPath('catalog', 'environment')],
  [internalPath('httpapi'), internalPath('transport', 'http')],
  [internalPath('localworkspace'), internalPath('local', 'workspace')],
  [internalPath('serverconfig'), internalPath('bootstrap', 'serverconfig')],
  [internalPath('store', 'postgres'), internalPath('persistence', 'postgres')],
  [internalPath('workbench'), internalPath('local', 'workbench')],
]);

function currentScenarioCommand(command) {
  return command.map((value) => packagePaths.get(value) ?? value);
}

if (report.schema_version !== '1.0' || report.scope !== 'deterministic_release_contract' || report.status !== 'passed') {
  failures.push('evaluation report identity or status is invalid');
}
if (report.plugin?.id !== 'contentcloud-video-production' || report.plugin?.version !== version || report.plugin?.digest !== `sha256:${pluginDigest}`) {
  failures.push('evaluation report is not bound to the current Plugin ID, version, and digest');
}
if (!Array.isArray(report.scenarios) || report.scenarios.length < 8) {
  failures.push('evaluation report must contain all required deterministic scenarios');
}

for (const scenario of report.scenarios ?? []) {
  if (!Array.isArray(scenario.command) || scenario.command.length < 2 || scenario.command[0] !== 'go' || scenario.command[1] !== 'test') {
    failures.push(`${scenario.id}: only structured go test commands are allowed`);
    continue;
  }
  const command = currentScenarioCommand(scenario.command);
  const result = spawnSync(command[0], command.slice(1), { cwd: root, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
  if (result.status !== 0) {
    failures.push(`${scenario.id}: ${result.stderr || result.stdout || `exit ${result.status}`}`.trim());
    continue;
  }
  for (const evidence of scenario.evidence ?? []) {
    if (!result.stdout.includes(evidence)) {
      failures.push(`${scenario.id}: evidence test ${evidence} did not run`);
    }
  }
}

const result = {
  ok: failures.length === 0,
  plugin: report.plugin,
  scope: report.scope,
  scenario_count: report.scenarios?.length ?? 0,
  limitations: report.limitations,
  errors: failures,
};
process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
if (failures.length > 0) process.exit(1);
