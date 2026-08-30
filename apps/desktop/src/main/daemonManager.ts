import { app } from 'electron';
import { access, mkdir, readFile, stat } from 'node:fs/promises';
import { constants } from 'node:fs';
import { join, resolve } from 'node:path';
import { execFile, spawn, type ChildProcess } from 'node:child_process';

import type { DesktopDaemonStatus } from '../shared/contracts';

type DiscoveryFile = {
  schema_version: 'contentcloud.desktop-api-discovery/1.0';
  endpoint: string;
  capability: string;
  api_versions: string[];
  pid: number;
};

const discoverySchema = 'contentcloud.desktop-api-discovery/1.0';
const apiVersion = '1.0';
const launchAgentLabel = 'com.goodvision.contentcloud';
const inheritedEnvironment = [
  'PATH', 'LANG', 'LC_ALL', 'TMPDIR', 'CODEX_HOME', 'CLAUDE_CONFIG_DIR', 'XDG_CONFIG_HOME',
  'SSL_CERT_FILE', 'SSL_CERT_DIR', 'AWS_PROFILE', 'AWS_REGION', 'AWS_DEFAULT_REGION', 'AWS_CONFIG_FILE',
  'AWS_SHARED_CREDENTIALS_FILE', 'GOOGLE_APPLICATION_CREDENTIALS', 'CLAUDE_CODE_USE_BEDROCK', 'CLAUDE_CODE_USE_VERTEX',
  'CONTENTCLOUD_CONFIG_PATH',
];

let child: ChildProcess | undefined;
let childExit: Promise<number | null> | undefined;
let lastFailure: string | undefined;
let operation: Promise<DesktopDaemonStatus> | undefined;

function discoveryPath(): string {
  return join(app.getPath('appData'), 'contentcloud', 'desktop-api.json');
}

function logPath(): string {
  return join(app.getPath('appData'), 'contentcloud', 'desktop-daemon.log');
}

function candidateExecutables(): string[] {
  const explicit = process.env.CONTENTCLOUD_DESKTOP_CLI_PATH?.trim();
  const packaged = join(process.resourcesPath, process.platform === 'win32' ? 'contentcloud.exe' : 'contentcloud');
  const appRoot = app.getAppPath();
  const development = resolve(appRoot, '../../bin', process.platform === 'win32' ? 'contentcloud.exe' : 'contentcloud');
  const workingTree = resolve(process.cwd(), 'bin', process.platform === 'win32' ? 'contentcloud.exe' : 'contentcloud');
  return [explicit, packaged, development, workingTree].filter((value): value is string => Boolean(value));
}

async function resolveExecutable(): Promise<string> {
  for (const candidate of candidateExecutables()) {
    try {
      const info = await stat(candidate);
      if (info.isFile()) {
        await access(candidate, constants.X_OK);
        return candidate;
      }
    } catch {
      // Try the next fixed candidate. PATH lookup is intentionally not used.
    }
  }
  throw new Error('未找到随应用发布或开发环境配置的 Content Work OS CLI');
}

async function readDiscovery(): Promise<DiscoveryFile | undefined> {
  try {
    const value: unknown = JSON.parse(await readFile(discoveryPath(), 'utf8'));
    if (!value || typeof value !== 'object') return undefined;
    const candidate = value as Partial<DiscoveryFile>;
    if (candidate.schema_version !== discoverySchema || typeof candidate.endpoint !== 'string' || !candidate.endpoint.startsWith('http://127.0.0.1:')) return undefined;
    if (typeof candidate.capability !== 'string' || candidate.capability.length < 32 || !Array.isArray(candidate.api_versions) || !candidate.api_versions.includes(apiVersion)) return undefined;
    if (!Number.isSafeInteger(candidate.pid) || Number(candidate.pid) <= 0) return undefined;
    return candidate as DiscoveryFile;
  } catch {
    return undefined;
  }
}

async function probe(): Promise<{ discovery: DiscoveryFile; version: string } | undefined> {
  const discovery = await readDiscovery();
  if (!discovery) return undefined;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 900);
  try {
    const response = await fetch(`${discovery.endpoint}/v1/health`, {
      headers: { Authorization: `Bearer ${discovery.capability}` },
      signal: controller.signal,
    });
    if (!response.ok) return undefined;
    const value: unknown = await response.json();
    const version = value && typeof value === 'object' && typeof (value as { version?: unknown }).version === 'string'
      ? (value as { version: string }).version
      : 'unknown';
    return { discovery, version };
  } catch {
    return undefined;
  } finally {
    clearTimeout(timer);
  }
}

function status(state: DesktopDaemonStatus['state'], options: Partial<DesktopDaemonStatus> = {}): DesktopDaemonStatus {
  return { state, managed: Boolean(child && child.exitCode === null && child.signalCode === null), ...options };
}

async function currentStatus(): Promise<DesktopDaemonStatus> {
  const live = await probe();
  if (live) {
    return status('running', { version: live.version, pid: live.discovery.pid, managed: Boolean(child && live.discovery.pid === child.pid), message: child && live.discovery.pid === child.pid ? undefined : 'Daemon 由系统服务托管' });
  }
  if (child && child.exitCode === null && child.signalCode === null) return status('starting', { pid: child.pid, message: 'Daemon 正在启动' });
  if (lastFailure) return status('failed', { message: lastFailure });
  return status('stopped', { message: 'Daemon 未运行' });
}

async function hasLocalBinding(): Promise<boolean> {
  try {
    const configuredPath = process.env.CONTENTCLOUD_CONFIG_PATH?.trim();
    const path = configuredPath || join(app.getPath('appData'), 'contentcloud', 'config.json');
    const body = JSON.parse(await readFile(path, 'utf8')) as { daemon_bindings?: unknown };
    return Array.isArray(body.daemon_bindings) && body.daemon_bindings.length > 0;
  } catch {
    return false;
  }
}

async function waitForStatus(timeoutMs: number): Promise<DesktopDaemonStatus> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const result = await currentStatus();
    if (result.state === 'running' || result.state === 'failed') return result;
    await new Promise((resolvePromise) => setTimeout(resolvePromise, 180));
  }
  return currentStatus();
}

function runLaunchctl(...args: string[]): Promise<void> {
  return new Promise((resolvePromise, rejectPromise) => {
    if (process.platform !== 'darwin' || typeof process.getuid !== 'function') {
      rejectPromise(new Error('当前平台不支持 LaunchAgent 控制'));
      return;
    }
    execFile('/bin/launchctl', args, { timeout: 5_000 }, (error) => error ? rejectPromise(error) : resolvePromise());
  });
}

function launchAgentTarget(): string | undefined {
  return typeof process.getuid === 'function' ? `gui/${process.getuid()}/${launchAgentLabel}` : undefined;
}

function daemonEnvironment(): NodeJS.ProcessEnv {
  const environment: NodeJS.ProcessEnv = { HOME: app.getPath('home'), PATH: '/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin' };
  for (const key of inheritedEnvironment) {
    const value = process.env[key]?.trim();
    if (value) environment[key] = value;
  }
  return environment;
}

function spawnDaemon(executable: string): void {
  child = spawn(executable, ['daemon', 'run', '--log-file', logPath()], {
    cwd: app.getPath('userData'),
    env: daemonEnvironment(),
    stdio: ['ignore', 'ignore', 'pipe'],
    windowsHide: true,
  });
  child.stderr?.on('data', () => undefined);
  childExit = new Promise((resolveExit) => child?.once('exit', (code) => resolveExit(code)));
  child.once('error', () => { lastFailure = '无法启动 Daemon 进程'; });
  child.once('exit', (code) => {
    if (code !== 0 && !lastFailure) lastFailure = `Daemon 进程已退出（代码 ${code ?? 'unknown'}）`;
    child = undefined;
  });
}

async function startInternal(): Promise<DesktopDaemonStatus> {
  const before = await currentStatus();
  if (before.state === 'running') return before;
  if (!(await hasLocalBinding())) return status('stopped', { message: '完成设备绑定后，Desktop 才能启动 Daemon' });
  lastFailure = undefined;
  let executable: string;
  try {
    executable = await resolveExecutable();
    await mkdir(join(app.getPath('appData'), 'contentcloud'), { recursive: true, mode: 0o700 });
  } catch (error) {
    return status('failed', { message: error instanceof Error ? error.message : '无法准备 Daemon' });
  }
  spawnDaemon(executable);
  return waitForStatus(8_000);
}

function serialized(action: () => Promise<DesktopDaemonStatus>): Promise<DesktopDaemonStatus> {
  if (!operation) operation = action().finally(() => { operation = undefined; });
  return operation;
}

export function getDaemonStatus(): Promise<DesktopDaemonStatus> {
  return currentStatus();
}

export function startDaemon(): Promise<DesktopDaemonStatus> {
  return serialized(startInternal);
}

export function stopDaemon(): Promise<DesktopDaemonStatus> {
  return serialized(stopInternal);
}

async function stopInternal(): Promise<DesktopDaemonStatus> {
    if (child && child.exitCode === null && child.signalCode === null) {
      const target = child;
      target.kill('SIGTERM');
      await Promise.race([childExit ?? Promise.resolve(null), new Promise((resolvePromise) => setTimeout(resolvePromise, 4_000))]);
      if (child && child === target && target.exitCode === null && target.signalCode === null) target.kill('SIGKILL');
      child = undefined;
      return currentStatus();
    }
    const live = await probe();
    const target = launchAgentTarget();
    if (live && target) {
      try { await runLaunchctl('bootout', target); } catch { /* already stopped is harmless */ }
      return waitForStatus(4_000);
    }
    return currentStatus();
}

export function restartDaemon(): Promise<DesktopDaemonStatus> {
  return serialized(async () => {
    if (child && child.exitCode === null && child.signalCode === null) await stopInternal();
    const live = await probe();
    const target = launchAgentTarget();
    if (live && target) {
      try {
        await runLaunchctl('kickstart', '-k', target);
        return waitForStatus(4_000);
      } catch {
        return status('failed', { message: '无法重启系统托管的 Daemon' });
      }
    }
    return startInternal();
  });
}

export function shutdownDaemon(): void {
  if (child && child.exitCode === null && child.signalCode === null) child.kill('SIGTERM');
}
