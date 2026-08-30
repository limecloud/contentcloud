import { safeStorage, app } from 'electron';
import { chmod, mkdir, readFile, unlink, writeFile } from 'node:fs/promises';
import { join } from 'node:path';

import type { DesktopAuthResult, DesktopLoginInput, DesktopAuthSession } from '../shared/contracts';

interface ServerEnvelope<T> {
  ok?: boolean;
  data?: T;
  error?: { code?: unknown; message?: unknown };
}

interface SessionPayload {
  user?: { id?: unknown; email?: unknown; display_name?: unknown };
  tenant?: { id?: unknown; name?: unknown };
  role?: unknown;
  is_platform_admin?: unknown;
}

interface AuthState {
  serverURL: string;
  cookie: string;
  session: DesktopAuthSession;
}

export interface AuthenticatedRequestResult<T> {
  value: T;
  response: Response;
}

export interface AuthenticatedBinaryResult {
  data: Uint8Array;
  response: Response;
}

let state: AuthState | undefined;

function authFilePath(): string {
  return join(app.getPath('userData'), 'auth-session.bin');
}

async function persistAuth(): Promise<void> {
  if (!state || !safeStorage.isEncryptionAvailable()) return;
  const path = authFilePath();
  await mkdir(app.getPath('userData'), { recursive: true, mode: 0o700 });
  const encrypted = safeStorage.encryptString(JSON.stringify(state));
  await writeFile(path, encrypted, { mode: 0o600 });
  await chmod(path, 0o600);
}

async function clearPersistedAuth(): Promise<void> {
  try { await unlink(authFilePath()); } catch { /* missing session is already logged out */ }
}

export async function restoreAuth(): Promise<void> {
  if (!safeStorage.isEncryptionAvailable()) return;
  try {
    const encrypted = await readFile(authFilePath());
    const candidate = JSON.parse(safeStorage.decryptString(encrypted)) as Partial<AuthState>;
    if (typeof candidate.serverURL !== 'string' || typeof candidate.cookie !== 'string' || !candidate.session) return;
    state = candidate as AuthState;
  } catch {
    await clearPersistedAuth();
  }
}

function normalizeServerURL(value: string): string {
  const parsed = new URL(value.trim());
  if (parsed.protocol !== 'https:' && !(parsed.protocol === 'http:' && ['127.0.0.1', 'localhost'].includes(parsed.hostname))) {
    throw new Error('服务端地址必须使用 HTTPS（本机开发环境可使用 HTTP）');
  }
  if (parsed.username || parsed.password || parsed.search || parsed.hash) throw new Error('服务端地址不能包含账号、查询参数或片段');
  return parsed.toString().replace(/\/$/, '');
}

function cookieFromResponse(response: Response): string {
  const raw = response.headers.get('set-cookie') ?? '';
  const match = raw.match(/(?:^|,\s*)cc_session=([^;,\s]+)/);
  return match?.[1] ? `cc_session=${match[1]}` : '';
}

async function readEnvelope<T>(response: Response): Promise<ServerEnvelope<T>> {
  const body = await response.text();
  let parsed: ServerEnvelope<T>;
  try {
    parsed = JSON.parse(body) as ServerEnvelope<T>;
  } catch {
    throw new Error(`服务端返回了无效响应（HTTP ${response.status}）`);
  }
  if (!response.ok || parsed.ok !== true) {
    const code = typeof parsed.error?.code === 'string' ? parsed.error.code : 'SERVER_REQUEST_FAILED';
    const message = typeof parsed.error?.message === 'string' ? parsed.error.message : `服务端请求失败（HTTP ${response.status}）`;
    const error = Object.assign(new Error(message), { code, status: response.status });
    throw error;
  }
  return parsed;
}

async function request<T>(serverURL: string, path: string, init: RequestInit = {}, cookie?: string, timeoutMs = 4_000): Promise<{ value: T; response: Response }> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const headers = new Headers(init.headers);
    headers.set('Accept', 'application/json');
    if (cookie) headers.set('Cookie', cookie);
    const response = await fetch(`${serverURL}${path}`, { ...init, headers, signal: controller.signal });
    const envelope = await readEnvelope<T>(response);
    return { value: envelope.data as T, response };
  } finally {
    clearTimeout(timer);
  }
}

async function requestBinary(serverURL: string, path: string, init: RequestInit = {}, cookie?: string, timeoutMs = 20_000): Promise<AuthenticatedBinaryResult> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const headers = new Headers(init.headers);
    headers.set('Accept', '*/*');
    if (cookie) headers.set('Cookie', cookie);
    const response = await fetch(`${serverURL}${path}`, { ...init, headers, signal: controller.signal });
    if (!response.ok) {
      const error = Object.assign(new Error(`服务端请求失败（HTTP ${response.status}）`), { status: response.status, code: 'SERVER_REQUEST_FAILED' });
      throw error;
    }
    return { data: new Uint8Array(await response.arrayBuffer()), response };
  } finally {
    clearTimeout(timer);
  }
}

/**
 * Execute a request with the cookie held by the Electron main process.
 * Renderer code never receives this cookie; serverGateway only receives the
 * decoded response envelope.
 */
export async function requestAuthenticated<T>(path: string, init: RequestInit = {}, timeoutMs = 4_000): Promise<AuthenticatedRequestResult<T>> {
  if (!state) {
    const error = Object.assign(new Error('尚未登录服务端'), { code: 'AUTH_REQUIRED', status: 401 });
    throw error;
  }
  return request<T>(state.serverURL, path, init, state.cookie, timeoutMs);
}

export async function requestAuthenticatedBinary(path: string, timeoutMs = 20_000): Promise<AuthenticatedBinaryResult> {
  if (!state) {
    const error = Object.assign(new Error('尚未登录服务端'), { code: 'AUTH_REQUIRED', status: 401 });
    throw error;
  }
  return requestBinary(state.serverURL, path, {}, state.cookie, timeoutMs);
}

function sessionFromPayload(serverURL: string, payload: SessionPayload): DesktopAuthSession {
  const user = payload.user;
  const tenant = payload.tenant;
  if (!user || typeof user.email !== 'string' || typeof user.display_name !== 'string' || !tenant || typeof tenant.name !== 'string' || typeof payload.role !== 'string') {
    throw new Error('服务端会话数据不完整');
  }
  return {
    server_url: serverURL,
    user: { id: typeof user.id === 'string' ? user.id : undefined, email: user.email, display_name: user.display_name },
    tenant: { id: typeof tenant.id === 'string' ? tenant.id : undefined, name: tenant.name },
    role: payload.role,
    is_platform_admin: payload.is_platform_admin === true,
  };
}

export async function getAuthSession(): Promise<DesktopAuthResult> {
  if (!state) return { status: 'unauthenticated' };
  try {
    const result = await request<SessionPayload>(state.serverURL, '/api/bff/session', {}, state.cookie);
    state.session = { ...state.session, ...sessionFromPayload(state.serverURL, result.value) };
    return { status: 'authenticated', session: state.session };
  } catch (error) {
    if (error instanceof Error && 'status' in error && (error as { status?: number }).status === 401) {
      state = undefined;
      await clearPersistedAuth();
      return { status: 'unauthenticated' };
    }
    return { status: 'offline', message: error instanceof Error ? error.message : '服务端暂不可用' };
  }
}

export async function login(input: DesktopLoginInput): Promise<DesktopAuthResult> {
  try {
    const serverURL = normalizeServerURL(input.server_url);
    const email = input.email.trim();
    if (!email || !input.password) return { status: 'rejected', code: 'AUTH_INPUT_INVALID', message: '请输入邮箱和密码' };
    const result = await request<unknown>(serverURL, '/api/v1/auth/login', {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email, password: input.password }),
    });
    const cookie = cookieFromResponse(result.response);
    if (!cookie) return { status: 'rejected', code: 'AUTH_COOKIE_MISSING', message: '服务端没有返回有效会话' };
    const sessionResult = await request<SessionPayload>(serverURL, '/api/bff/session', {}, cookie);
    const session = sessionFromPayload(serverURL, sessionResult.value);
    const loginPayload = result.value && typeof result.value === 'object' ? result.value as { expires_at?: unknown } : {};
    if (typeof loginPayload.expires_at === 'string') session.expires_at = loginPayload.expires_at;
    state = { serverURL, cookie, session };
    await persistAuth();
    return { status: 'authenticated', session };
  } catch (error) {
    const value = error as { code?: unknown; status?: unknown; message?: unknown };
    if (typeof value.code === 'string' && typeof value.message === 'string') return { status: 'rejected', code: value.code, message: value.message };
    return { status: 'offline', message: error instanceof Error ? error.message : '服务端暂不可用' };
  }
}

export async function logout(): Promise<DesktopAuthResult> {
  const current = state;
  state = undefined;
  await clearPersistedAuth();
  if (!current) return { status: 'unauthenticated' };
  try {
    await request<unknown>(current.serverURL, '/api/bff/session/logout', { method: 'POST' }, current.cookie);
    return { status: 'unauthenticated' };
  } catch (error) {
    return { status: 'offline', message: error instanceof Error ? error.message : '退出登录失败' };
  }
}
