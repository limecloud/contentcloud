export interface ApiError { code: string; message: string; hint?: string }
interface Envelope<T> { ok: boolean; data?: T; error?: ApiError }

function responseContentType(response: Response): string {
  return response.headers.get('Content-Type') || '未知类型';
}

function responsePreview(body: string): string {
  const preview = body.replace(/\s+/g, ' ').trim();
  if (!preview) return '响应为空';
  return preview.length > 160 ? `${preview.slice(0, 160)}…` : preview;
}

function invalidResponseError(path: string, response: Response, body: string, reason?: unknown): Error {
  const message = `接口 ${path} 返回了无效 JSON（HTTP ${response.status}，Content-Type: ${responseContentType(response)}）：${responsePreview(body)}`;
  const error = Object.assign(new Error(message), {
    api: {
      code: 'INVALID_JSON_RESPONSE',
      message,
      hint: reason instanceof Error ? reason.message : undefined
    },
    status: response.status
  });
  return error;
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const hasBody = init?.body !== undefined;
  const isForm = typeof FormData !== 'undefined' && init?.body instanceof FormData;
  const response = await fetch(path, {
    credentials: 'same-origin',
    headers: {...(hasBody && !isForm ? {'Content-Type': 'application/json'} : {}), ...(init?.headers || {})},
    ...init
  });
  let body: Envelope<T> | null = null;
  if (response.status !== 204) {
    const raw = await response.text();
    try {
      body = JSON.parse(raw) as Envelope<T>;
    } catch (reason) {
      throw invalidResponseError(path, response, raw, reason);
    }
  }
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
