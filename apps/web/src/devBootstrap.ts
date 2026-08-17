/// <reference types="vite/client" />

import { postWithoutBody } from './api';

export async function bootstrapDevelopmentSession(): Promise<boolean> {
  if (!import.meta.env.DEV) return false;
  await postWithoutBody('/api/v1/dev/bootstrap');
  return true;
}
