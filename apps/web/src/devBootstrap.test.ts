import { afterEach, describe, expect, it, vi } from 'vitest';

afterEach(() => {
  vi.resetModules();
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe('development session bootstrap', () => {
  it('does not call the development endpoint in a production build', async () => {
    vi.stubEnv('DEV', false);
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    const { bootstrapDevelopmentSession } = await import('./devBootstrap');

    await expect(bootstrapDevelopmentSession()).resolves.toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('bootstraps a local development session only in a development build', async () => {
    vi.stubEnv('DEV', true);
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ok: true, data: {ready: true}}),
      {status: 200, headers: {'Content-Type': 'application/json'}}
    )));
    const { bootstrapDevelopmentSession } = await import('./devBootstrap');

    await expect(bootstrapDevelopmentSession()).resolves.toBe(true);
  });
});
