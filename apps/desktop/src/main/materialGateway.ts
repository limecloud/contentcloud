import { dialog } from 'electron';
import { randomUUID } from 'node:crypto';
import { readFile, stat } from 'node:fs/promises';
import { basename, extname } from 'node:path';

import type { DesktopFileSelection, DesktopFileSelectionResult, DesktopMaterialUploadResult, DesktopWorkspaceMaterial } from '../shared/contracts';
import { requestAuthenticated, requestAuthenticatedBinary } from './authGateway';

const maxFileBytes = 100 * 1024 * 1024;
const maxSelection = 50;
const selectionTTL = 15 * 60 * 1000;
const maxPreviewBytes = 8 * 1024 * 1024;

const mimeByExtension: Record<string, string> = {
  '.pdf': 'application/pdf',
  '.docx': 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  '.xlsx': 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  '.pptx': 'application/vnd.openxmlformats-officedocument.presentationml.presentation',
  '.png': 'image/png', '.jpg': 'image/jpeg', '.jpeg': 'image/jpeg',
  '.mp4': 'video/mp4', '.mov': 'video/quicktime', '.webm': 'video/webm',
  '.mp3': 'audio/mpeg', '.wav': 'audio/wav', '.m4a': 'audio/mp4',
  '.csv': 'text/csv', '.html': 'text/html', '.htm': 'text/html',
  '.json': 'application/json', '.md': 'text/markdown', '.txt': 'text/plain',
};

interface PendingFile {
  id: string;
  path: string;
  name: string;
  size: number;
  mimeType: string;
  expiresAt: number;
}

const pendingFiles = new Map<string, PendingFile>();

function prunePendingFiles(): void {
  const now = Date.now();
  for (const [id, file] of pendingFiles) {
    if (file.expiresAt <= now) pendingFiles.delete(id);
  }
}

function selectionFile(file: PendingFile): DesktopFileSelection {
  return { id: file.id, name: file.name, size: file.size, mime_type: file.mimeType };
}

export async function chooseFiles(): Promise<DesktopFileSelectionResult> {
  prunePendingFiles();
  const result = await dialog.showOpenDialog({
    properties: ['openFile', 'multiSelections'],
    filters: [{ name: '项目资料', extensions: Object.keys(mimeByExtension).map((value) => value.slice(1)) }],
  });
  if (result.canceled || result.filePaths.length === 0) return { status: 'canceled', files: [] };
  if (result.filePaths.length > maxSelection) return { status: 'rejected', files: [], message: `一次最多选择 ${maxSelection} 个文件` };

  const selected: DesktopFileSelection[] = [];
  for (const path of result.filePaths) {
    try {
      const info = await stat(path);
      const name = basename(path);
      const mimeType = mimeByExtension[extname(name).toLowerCase()];
      if (!info.isFile() || info.size <= 0 || info.size > maxFileBytes || !mimeType) continue;
      const id = `file_${randomUUID()}`;
      const file: PendingFile = { id, path, name, size: info.size, mimeType, expiresAt: Date.now() + selectionTTL };
      pendingFiles.set(id, file);
      selected.push(selectionFile(file));
    } catch {
      // A file can disappear between the native picker and stat; omit it from the queue.
    }
  }
  if (selected.length === 0) return { status: 'rejected', files: [], message: '没有可上传的文件（支持的文件不超过 100MB）' };
  return { status: 'selected', files: selected };
}

function uploadError(error: unknown): string {
  if (error && typeof error === 'object' && typeof (error as { message?: unknown }).message === 'string') return (error as { message: string }).message;
  return '上传失败';
}

function isMaterial(value: unknown): value is DesktopWorkspaceMaterial {
  if (!value || typeof value !== 'object') return false;
  const material = value as Partial<DesktopWorkspaceMaterial>;
  return typeof material.ref === 'string' && material.ref.length > 0
    && typeof material.project_id === 'string' && material.project_id.length > 0
    && typeof material.title === 'string' && typeof material.file_name === 'string'
    && typeof material.mime_type === 'string' && typeof material.byte_size === 'number';
}

export async function uploadMaterials(projectID: string, fileIDs: string[]): Promise<DesktopMaterialUploadResult[]> {
  prunePendingFiles();
  const ids = [...new Set(fileIDs)].slice(0, maxSelection);
  const results: DesktopMaterialUploadResult[] = [];
  for (const fileID of ids) {
    const pending = pendingFiles.get(fileID);
    if (!pending) {
      results.push({ file_id: fileID, file_name: '未知文件', status: 'rejected', message: '文件选择已过期，请重新选择' });
      continue;
    }
    try {
      const data = await readFile(pending.path);
      if (data.byteLength !== pending.size || data.byteLength > maxFileBytes) throw new Error('文件在上传前发生变化，请重新选择');
      const form = new FormData();
      form.append('project_id', projectID);
      const extension = extname(pending.name);
      form.append('title', pending.name.slice(0, extension ? -extension.length : undefined));
      form.append('file_type', pending.mimeType);
      form.append('file', new Blob([data], { type: pending.mimeType }), pending.name);
      const response = await requestAuthenticated<unknown>('/api/studio/materials', { method: 'POST', body: form }, 10 * 60 * 1000);
      if (!isMaterial(response.value)) throw new Error('服务端返回的素材数据格式不受支持');
      results.push({ file_id: fileID, file_name: pending.name, status: 'uploaded', material: response.value });
      pendingFiles.delete(fileID);
    } catch (error) {
      const status = error && typeof error === 'object' && typeof (error as { status?: unknown }).status === 'number' ? (error as { status: number }).status : undefined;
      results.push({ file_id: fileID, file_name: pending.name, status: status && status >= 500 ? 'offline' : 'rejected', message: uploadError(error) });
    }
  }
  return results;
}

export async function previewMaterial(materialRef: string) {
  const id = materialRef.trim().replace(/^material:/, '');
  if (!id || id.length > 256) return { status: 'rejected', message: '素材引用无效' } as const;
  try {
    const response = await requestAuthenticatedBinary(`/api/studio/materials/${encodeURIComponent(id)}/download`);
    if (response.data.byteLength === 0 || response.data.byteLength > maxPreviewBytes) return { status: 'rejected', message: '文件过大，暂不支持内嵌预览' } as const;
    const mimeType = response.response.headers.get('content-type')?.split(';', 1)[0]?.trim() || 'application/octet-stream';
    if (!mimeType.startsWith('image/') && !mimeType.startsWith('video/') && !mimeType.startsWith('audio/')) return { status: 'rejected', message: '该文件类型暂不支持内嵌预览' } as const;
    const disposition = response.response.headers.get('content-disposition') ?? '';
    const fileName = disposition.match(/filename="?([^";]+)"?/i)?.[1] ?? id;
    return { status: 'ready', data_url: `data:${mimeType};base64,${Buffer.from(response.data).toString('base64')}`, mime_type: mimeType, file_name: fileName } as const;
  } catch (error) {
    const status = error && typeof error === 'object' && typeof (error as { status?: unknown }).status === 'number' ? (error as { status: number }).status : undefined;
    if (status === 401) return { status: 'rejected', message: '请先登录服务端后再预览素材' } as const;
    return { status: status && status >= 500 ? 'offline' : 'rejected', message: uploadError(error) } as const;
  }
}
