import {
  isServerBootstrap,
  isServerConnectSession,
  type DesktopServerBootstrap,
  type DesktopServerConnectSession,
  type DesktopServerResult,
  type DesktopCreateServerProjectInput,
  type DesktopAssetSurface,
  type DesktopDeliveries,
  isDesktopAssetSurface,
  isDesktopDeliveries,
} from '../shared/contracts';
import { requestAuthenticated } from './authGateway';

function errorDetails(error: unknown): { status?: number; code?: string; message: string } {
  if (!error || typeof error !== 'object') return { message: '服务端暂不可用' };
  const value = error as { status?: unknown; code?: unknown; message?: unknown };
  return {
    status: typeof value.status === 'number' ? value.status : undefined,
    code: typeof value.code === 'string' ? value.code : undefined,
    message: typeof value.message === 'string' ? value.message : '服务端暂不可用',
  };
}

async function serverRequest<T>(path: string, validator: (value: unknown) => value is T, init?: RequestInit): Promise<DesktopServerResult<T>> {
  try {
    const result = await requestAuthenticated<T>(path, init);
    if (!validator(result.value)) return { status: 'offline', message: '服务端返回的数据格式不受支持' };
    return { status: 'ready', value: result.value };
  } catch (error) {
    const details = errorDetails(error);
    if (details.status === 401 || details.code === 'AUTH_REQUIRED') return { status: 'unauthenticated' };
    if (details.status !== undefined && details.status >= 400 && details.status < 500) {
      return { status: 'rejected', code: details.code ?? 'SERVER_REQUEST_REJECTED', message: details.message };
    }
    return { status: 'offline', message: details.message };
  }
}

export function getServerBootstrap(): Promise<DesktopServerResult<DesktopServerBootstrap>> {
  return serverRequest('/api/studio/bootstrap', isServerBootstrap);
}

export function getServerAssets(projectID: string): Promise<DesktopServerResult<DesktopAssetSurface>> {
  return serverRequest(`/api/studio/assets?project_id=${encodeURIComponent(projectID)}`, isDesktopAssetSurface);
}

export function getServerDeliveries(): Promise<DesktopServerResult<DesktopDeliveries>> {
  return serverRequest('/api/studio/deliveries', isDesktopDeliveries);
}

export async function createServerProject(input: DesktopCreateServerProjectInput): Promise<DesktopServerResult<DesktopServerBootstrap>> {
  try {
    await requestAuthenticated('/api/bff/projects', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    });
  } catch (error) {
    const details = errorDetails(error);
    if (details.status === 401 || details.code === 'AUTH_REQUIRED') return { status: 'unauthenticated' };
    if (details.status !== undefined && details.status >= 400 && details.status < 500) return { status: 'rejected', code: details.code ?? 'PROJECT_CREATE_REJECTED', message: details.message };
    return { status: 'offline', message: details.message };
  }
  return getServerBootstrap();
}

export function createConnectSession(projectID: string): Promise<DesktopServerResult<DesktopServerConnectSession>> {
  return serverRequest(`/api/studio/projects/${encodeURIComponent(projectID)}/connect-sessions`, isServerConnectSession, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: '{}',
  });
}

export function getConnectSession(sessionID: string): Promise<DesktopServerResult<DesktopServerConnectSession>> {
  return serverRequest(`/api/studio/connect-sessions/${encodeURIComponent(sessionID)}`, isServerConnectSession);
}

export function cancelConnectSession(sessionID: string): Promise<DesktopServerResult<DesktopServerConnectSession>> {
  return serverRequest(`/api/studio/connect-sessions/${encodeURIComponent(sessionID)}/cancel`, isServerConnectSession, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: '{}',
  });
}
