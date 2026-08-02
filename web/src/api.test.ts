import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('API response parsing', () => {
  it('returns data from the JSON envelope', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ok: true, data: {id: 'project-1'}}),
      {status: 200, headers: {'Content-Type': 'application/json'}}
    )));

    await expect(api<{id: string}>('/api/bff/projects/project-1')).resolves.toEqual({id: 'project-1'});
  });

  it('reports non-JSON responses without exposing the parser SyntaxError', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(
      '404 page not found\n',
      {status: 404, headers: {'Content-Type': 'text/plain; charset=utf-8'}}
    )));

    const request = api('/api/bff/input-items');
    await expect(request).rejects.toMatchObject({
      status: 404,
      api: {code: 'INVALID_JSON_RESPONSE'}
    });
    await expect(request).rejects.toThrow('接口 /api/bff/input-items 返回了无效 JSON（HTTP 404，Content-Type: text/plain; charset=utf-8）：404 page not found');
    await expect(request).rejects.not.toThrow('Unexpected non-whitespace character after JSON');
  });

  it('keeps API errors from valid error envelopes', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ok: false, error: {code: 'SESSION_REQUIRED', message: '请先登录'}}),
      {status: 401, headers: {'Content-Type': 'application/json'}}
    )));

    await expect(api('/api/bff/session')).rejects.toMatchObject({
      status: 401,
      message: '请先登录',
      api: {code: 'SESSION_REQUIRED'}
    });
  });
});
