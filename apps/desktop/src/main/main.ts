import { app, BrowserWindow, ipcMain, session } from 'electron';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';

import { getAuthSession, login, logout, restoreAuth } from './authGateway';
import { addReviewComment, decideReview, publishWorkspace, requestProjectEvents, requestReviewInbox, requestReviewRevision, requestSnapshot, retryWorkspace } from './desktopGateway';
import { cancelConnectSession, createConnectSession, createServerProject, getConnectSession, getServerAssets, getServerBootstrap, getServerDeliveries } from './serverGateway';
import { chooseFiles, previewMaterial, uploadMaterials } from './materialGateway';
import { getDaemonStatus, restartDaemon, shutdownDaemon, startDaemon, stopDaemon } from './daemonManager';
import { downloadDeliveryArtifact } from './deliveryGateway';
import type { DesktopCreateServerProjectInput, DesktopLoginInput, DesktopSnapshotResult, PublishWorkspaceInput } from '../shared/contracts';
import { isCreateServerProjectInput, isDesktopLoginInput, isDesktopDaemonStatus, isPublishWorkspaceInput, isRetryWorkspaceInput, isReviewCommentRequest, isReviewDecisionRequest, isReviewRevisionRequest } from '../shared/contracts';

declare const MAIN_WINDOW_VITE_DEV_SERVER_URL: string | undefined;
declare const MAIN_WINDOW_VITE_NAME: string;

let mainWindow: BrowserWindow | null = null;
let pollTimer: NodeJS.Timeout | undefined;
let latestResult: DesktopSnapshotResult | undefined;
const projectEventCursors = new Map<string, number>();

function createWindow(): void {
  mainWindow = new BrowserWindow({
    width: 1440,
    height: 960,
    minWidth: 1080,
    minHeight: 720,
    backgroundColor: '#f8faff',
    webPreferences: {
      preload: join(__dirname, 'preload.js'),
      sandbox: true,
      contextIsolation: true,
      nodeIntegration: false,
      webSecurity: true,
    },
  });

  const rendererFile = join(__dirname, `../renderer/${MAIN_WINDOW_VITE_NAME}/index.html`);
  const allowedURL = MAIN_WINDOW_VITE_DEV_SERVER_URL ?? pathToFileURL(rendererFile).toString();
  mainWindow.webContents.on('will-navigate', (event, targetURL) => {
    const target = new URL(targetURL);
    const allowed = new URL(allowedURL);
    if (target.origin !== allowed.origin || target.pathname !== allowed.pathname) event.preventDefault();
  });
  mainWindow.webContents.setWindowOpenHandler(() => ({ action: 'deny' }));
  mainWindow.webContents.on('will-attach-webview', (event) => event.preventDefault());

  if (MAIN_WINDOW_VITE_DEV_SERVER_URL) {
    void mainWindow.loadURL(MAIN_WINDOW_VITE_DEV_SERVER_URL);
  } else {
    void mainWindow.loadURL(allowedURL);
  }
  void publishSnapshot();
  pollTimer = setInterval(() => void pollProjectEvents(), 2000);
}

async function publishSnapshot(): Promise<DesktopSnapshotResult> {
  const result = await requestSnapshot();
  latestResult = result;
  if (result.status === 'ready') {
    for (const project of result.snapshot.projects) projectEventCursors.set(project.project_id, project.event_cursor);
  }
  if (mainWindow && !mainWindow.isDestroyed()) mainWindow.webContents.send('desktop.snapshotChanged', result);
  return result;
}

async function pollProjectEvents(): Promise<void> {
  if (latestResult?.status !== 'ready') {
    await publishSnapshot();
    return;
  }
  const projects = latestResult.snapshot.projects;
  const results = await Promise.all(projects.map((project) => requestProjectEvents(project.project_id, projectEventCursors.get(project.project_id) ?? project.event_cursor)));
  if (results.some((result) => result.status === 'offline')) {
    await publishSnapshot();
    return;
  }
  const changed = results.some((result) => result.status === 'ready' && (result.stream.events.length > 0 || result.stream.resync_required));
  if (changed) await publishSnapshot();
}

app.whenReady().then(async () => {
  await restoreAuth();
  session.defaultSession.setPermissionRequestHandler((_webContents, _permission, callback) => callback(false));
  ipcMain.handle('desktop.snapshot', () => latestResult ?? publishSnapshot());
  ipcMain.handle('desktop.daemon.status', async () => {
    const result = await getDaemonStatus();
    return isDesktopDaemonStatus(result) ? result : { state: 'failed', managed: false, message: 'Daemon 状态格式无效' };
  });
  ipcMain.handle('desktop.daemon.start', async () => {
    const result = await startDaemon();
    if (result.state === 'running') await publishSnapshot();
    return isDesktopDaemonStatus(result) ? result : { state: 'failed', managed: false, message: 'Daemon 状态格式无效' };
  });
  ipcMain.handle('desktop.daemon.stop', async () => {
    const result = await stopDaemon();
    await publishSnapshot();
    return isDesktopDaemonStatus(result) ? result : { state: 'failed', managed: false, message: 'Daemon 状态格式无效' };
  });
  ipcMain.handle('desktop.daemon.restart', async () => {
    const result = await restartDaemon();
    if (result.state === 'running') await publishSnapshot();
    return isDesktopDaemonStatus(result) ? result : { state: 'failed', managed: false, message: 'Daemon 状态格式无效' };
  });
  ipcMain.handle('desktop.authSession', () => getAuthSession());
  ipcMain.handle('desktop.login', (_event, input: unknown) => {
    if (!isDesktopLoginInput(input)) return Promise.resolve({ status: 'rejected', code: 'AUTH_INPUT_INVALID', message: '登录参数无效' } as const);
    return login(input as DesktopLoginInput);
  });
  ipcMain.handle('desktop.logout', () => logout());
  ipcMain.handle('desktop.serverBootstrap', () => getServerBootstrap());
  ipcMain.handle('desktop.serverAssets', (_event, projectID: unknown) => {
    if (typeof projectID !== 'string' || projectID.trim().length === 0 || projectID.length > 256) return Promise.resolve({ status: 'rejected', code: 'DESKTOP_PROJECT_INVALID', message: '项目标识无效' } as const);
    return getServerAssets(projectID);
  });
  ipcMain.handle('desktop.serverDeliveries', () => getServerDeliveries());
  ipcMain.handle('desktop.delivery.download', (_event, artifactID: unknown, fileName: unknown) => {
    if (typeof artifactID !== 'string' || artifactID.trim().length === 0 || artifactID.length > 256 || typeof fileName !== 'string' || fileName.trim().length === 0 || fileName.length > 256) {
      return Promise.resolve({ status: 'rejected', file_name: typeof fileName === 'string' ? fileName.slice(0, 180) : '交付文件', message: '交付文件参数无效' } as const);
    }
    return downloadDeliveryArtifact(artifactID, fileName);
  });
  ipcMain.handle('desktop.files.choose', () => chooseFiles());
  ipcMain.handle('desktop.materials.upload', (_event, projectID: unknown, fileIDs: unknown) => {
    if (typeof projectID !== 'string' || projectID.trim().length === 0 || projectID.length > 256 || !Array.isArray(fileIDs) || fileIDs.some((value) => typeof value !== 'string' || value.length === 0 || value.length > 256)) return Promise.resolve([]);
    return uploadMaterials(projectID, fileIDs);
  });
  ipcMain.handle('desktop.materials.preview', (_event, materialRef: unknown) => {
    if (typeof materialRef !== 'string' || materialRef.trim().length === 0 || materialRef.length > 256) return Promise.resolve({ status: 'rejected', message: '素材引用无效' } as const);
    return previewMaterial(materialRef);
  });
  ipcMain.handle('desktop.serverProject.create', (_event, input: unknown) => {
    if (!isCreateServerProjectInput(input)) return Promise.resolve({ status: 'rejected', code: 'DESKTOP_PROJECT_INPUT_INVALID', message: '项目参数无效' } as const);
    return createServerProject(input as DesktopCreateServerProjectInput);
  });
  ipcMain.handle('desktop.connectSession.create', (_event, projectID: unknown) => {
    if (typeof projectID !== 'string' || projectID.trim().length === 0 || projectID.length > 256) return Promise.resolve({ status: 'rejected', code: 'DESKTOP_PROJECT_INVALID', message: '项目标识无效' } as const);
    return createConnectSession(projectID);
  });
  ipcMain.handle('desktop.connectSession.show', (_event, sessionID: unknown) => {
    if (typeof sessionID !== 'string' || sessionID.trim().length === 0 || sessionID.length > 256) return Promise.resolve({ status: 'rejected', code: 'DESKTOP_SESSION_INVALID', message: '连接会话标识无效' } as const);
    return getConnectSession(sessionID);
  });
  ipcMain.handle('desktop.connectSession.cancel', (_event, sessionID: unknown) => {
    if (typeof sessionID !== 'string' || sessionID.trim().length === 0 || sessionID.length > 256) return Promise.resolve({ status: 'rejected', code: 'DESKTOP_SESSION_INVALID', message: '连接会话标识无效' } as const);
    return cancelConnectSession(sessionID);
  });
  ipcMain.handle('desktop.projectEvents', (_event, projectID: unknown, after: unknown) => {
    if (typeof projectID !== 'string' || projectID.trim().length === 0 || projectID.length > 256 || typeof after !== 'number' || !Number.isSafeInteger(after) || after < 0) {
      return Promise.resolve({ status: 'offline', message: '项目事件查询参数无效' } as const);
    }
    return requestProjectEvents(projectID, after);
  });
  ipcMain.handle('desktop.publishWorkspace', async (_event, input: unknown) => {
    if (!isPublishWorkspaceInput(input)) return { status: 'rejected', code: 'DESKTOP_COMMAND_INPUT_INVALID' } as const;
    const result = await publishWorkspace(input as PublishWorkspaceInput);
    if (result.status === 'accepted') await publishSnapshot();
    return result;
  });
  ipcMain.handle('desktop.retryWorkspace', async (_event, input: unknown) => {
    if (!isRetryWorkspaceInput(input)) return { status: 'rejected', code: 'DESKTOP_COMMAND_INPUT_INVALID' } as const;
    const result = await retryWorkspace(input);
    if (result.status === 'accepted') await publishSnapshot();
    return result;
  });
  ipcMain.handle('desktop.appInfo', () => ({
    name: 'Content Work OS Desktop' as const,
    version: app.getVersion(),
    platform: process.platform,
    electron: process.versions.electron,
  }));
  ipcMain.handle('desktop.reviewInbox', (_event, projectID: unknown) => typeof projectID === 'string' ? requestReviewInbox(projectID) : Promise.resolve({ status: 'rejected', code: 'DESKTOP_PROJECT_INVALID' } as const));
  ipcMain.handle('desktop.reviewRevision', (_event, input: unknown) => {
    if (!isReviewRevisionRequest(input)) return Promise.resolve({ status: 'rejected', code: 'DESKTOP_REVIEW_INPUT_INVALID' } as const);
    const value = input;
    return requestReviewRevision(value.projectID, value.revisionID);
  });
  ipcMain.handle('desktop.reviewComment', (_event, input: unknown) => {
    if (!isReviewCommentRequest(input)) return Promise.resolve({ status: 'rejected', code: 'DESKTOP_REVIEW_INPUT_INVALID' } as const);
    const value = input;
    return addReviewComment(value.projectID, value.payload);
  });
  ipcMain.handle('desktop.reviewDecision', (_event, input: unknown) => {
    if (!isReviewDecisionRequest(input)) return Promise.resolve({ status: 'rejected', code: 'DESKTOP_REVIEW_INPUT_INVALID' } as const);
    const value = input;
    return decideReview(value.projectID, value.revisionID, value.action, value.payload ?? { reason: '' });
  });
  createWindow();
  void startDaemon().then(() => publishSnapshot());
  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow();
  });
});

app.on('window-all-closed', () => {
  if (pollTimer) clearInterval(pollTimer);
  if (process.platform !== 'darwin') app.quit();
});

app.on('before-quit', () => {
  shutdownDaemon();
});
