import { app, dialog } from 'electron';
import { writeFile } from 'node:fs/promises';
import { basename, join } from 'node:path';

import { requestAuthenticatedBinary } from './authGateway';
import type { DesktopDeliveryDownloadResult } from '../shared/contracts';

function safeFileName(value: string): string {
  const name = basename(value.trim()).replace(/[\u0000-\u001f]/g, '');
  return name.slice(0, 180) || 'contentcloud-delivery.bin';
}

function errorDetails(error: unknown): { status?: number; message: string } {
  if (!error || typeof error !== 'object') return { message: '交付文件暂不可用' };
  const value = error as { status?: unknown; message?: unknown };
  return { status: typeof value.status === 'number' ? value.status : undefined, message: typeof value.message === 'string' ? value.message : '交付文件暂不可用' };
}

export async function downloadDeliveryArtifact(artifactID: string, fileName: string): Promise<DesktopDeliveryDownloadResult> {
  const safeName = safeFileName(fileName);
  try {
    const result = await requestAuthenticatedBinary(`/api/studio/artifacts/${encodeURIComponent(artifactID)}/download`, 30_000);
    const selected = await dialog.showSaveDialog({
      title: '保存交付文件',
      defaultPath: join(app.getPath('downloads'), safeName),
      buttonLabel: '保存',
    });
    if (selected.canceled || !selected.filePath) return { status: 'canceled', file_name: safeName };
    await writeFile(selected.filePath, Buffer.from(result.data), { mode: 0o600 });
    return { status: 'saved', file_name: safeName };
  } catch (error) {
    const details = errorDetails(error);
    if (details.status === 401) return { status: 'unauthenticated', file_name: safeName };
    if (details.status !== undefined && details.status >= 400 && details.status < 500) return { status: 'rejected', file_name: safeName, message: details.message };
    return { status: 'offline', file_name: safeName, message: details.message };
  }
}
