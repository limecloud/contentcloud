import { app, BrowserWindow, ipcMain, session } from 'electron';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';

import { addReviewComment, decideReview, publishWorkspace, requestProjectEvents, requestReviewInbox, requestReviewRevision, requestSnapshot } from './desktopGateway';
import type { DesktopSnapshotResult, PublishWorkspaceInput } from '../shared/contracts';
import { isPublishWorkspaceInput, isReviewCommentRequest, isReviewDecisionRequest, isReviewRevisionRequest } from '../shared/contracts';

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

app.whenReady().then(() => {
  session.defaultSession.setPermissionRequestHandler((_webContents, _permission, callback) => callback(false));
  ipcMain.handle('desktop.snapshot', () => latestResult ?? publishSnapshot());
  ipcMain.handle('desktop.publishWorkspace', async (_event, input: unknown) => {
    if (!isPublishWorkspaceInput(input)) return { status: 'rejected', code: 'DESKTOP_COMMAND_INPUT_INVALID' } as const;
    const result = await publishWorkspace(input as PublishWorkspaceInput);
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
  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow();
  });
});

app.on('window-all-closed', () => {
  if (pollTimer) clearInterval(pollTimer);
  if (process.platform !== 'darwin') app.quit();
});
