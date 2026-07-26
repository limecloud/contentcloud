export interface ApiError { code: string; message: string; hint?: string }
interface Envelope<T> { ok: boolean; data?: T; error?: ApiError }

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const hasBody = init?.body !== undefined;
  const isForm = typeof FormData !== 'undefined' && init?.body instanceof FormData;
  const response = await fetch(path, {
    credentials: 'same-origin',
    headers: {...(hasBody && !isForm ? {'Content-Type': 'application/json'} : {}), ...(init?.headers || {})},
    ...init
  });
  const body = response.status === 204 ? null : await response.json() as Envelope<T>;
  if (!response.ok || !body?.ok) {
    const error = body?.error || {code: 'NETWORK_ERROR', message: `请求失败 (${response.status})`};
    throw Object.assign(new Error(error.message), {api: error, status: response.status});
  }
  return body.data as T;
}

export function post<T>(path: string, value: unknown = {}): Promise<T> {
  return api<T>(path, {method: 'POST', body: JSON.stringify(value)});
}

export function patch<T>(path: string, value: unknown): Promise<T> {
  return api<T>(path, {method: 'PATCH', body: JSON.stringify(value)});
}

export function upload<T>(path: string, form: FormData): Promise<T> {
  return api<T>(path, {method: 'POST', body: form});
}

export async function download(path: string): Promise<{blob: Blob; fileName: string}> {
  const response = await fetch(path, {credentials: 'same-origin'});
  if (!response.ok) {
    const body = await response.json().catch(() => null) as Envelope<unknown> | null;
    throw new Error(body?.error?.message || `下载失败 (${response.status})`);
  }
  const disposition = response.headers.get('Content-Disposition') || '';
  const match = disposition.match(/filename="?([^";]+)"?/i);
  return {blob: await response.blob(), fileName: match?.[1] || 'download'};
}
