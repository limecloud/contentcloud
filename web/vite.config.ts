import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';

declare const process: { env: Record<string, string | undefined> };

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', 'VITE_');
  const runtimeEnv = process.env;
  const configuredPort = parseInt(runtimeEnv.VITE_DEV_PORT ?? env.VITE_DEV_PORT ?? '', 10);
  const port = configuredPort >= 1 && configuredPort <= 65535 ? configuredPort : 5173;
  const proxyTarget = runtimeEnv.VITE_DEV_PROXY_TARGET || env.VITE_DEV_PROXY_TARGET || 'http://localhost:8080';

  return {
    plugins: [react()],
    server: {
      port,
      proxy: {
        '/api': proxyTarget,
        '/healthz': proxyTarget
      }
    }
  };
});
