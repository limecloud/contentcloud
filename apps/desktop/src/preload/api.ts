import type {
  DesktopAuthResult,
  DesktopDaemonStatus,
  DesktopCreateServerProjectInput,
  DesktopLoginInput,
  DesktopApi,
  DesktopAppInfo,
  DesktopCommandResult,
  DesktopDeliveryDownloadResult,
  DesktopEventStreamResult,
  DesktopReviewCommentInput,
  DesktopReviewDecisionInput,
  DesktopSnapshotResult,
  PublishWorkspaceInput,
  DesktopFileSelectionResult,
  DesktopMaterialUploadResult,
  DesktopMaterialPreviewResult,
} from '../shared/contracts';

export interface PreloadIPC {
  invoke(channel: string, ...args: unknown[]): Promise<unknown>;
  on(channel: string, listener: (...args: unknown[]) => void): void;
  removeListener(channel: string, listener: (...args: unknown[]) => void): void;
}

export function createDesktopApi(ipcRenderer: PreloadIPC): DesktopApi {
  return {
    getSnapshot: () => ipcRenderer.invoke('desktop.snapshot') as Promise<DesktopSnapshotResult>,
    getDaemonStatus: () => ipcRenderer.invoke('desktop.daemon.status') as Promise<DesktopDaemonStatus>,
    startDaemon: () => ipcRenderer.invoke('desktop.daemon.start') as Promise<DesktopDaemonStatus>,
    stopDaemon: () => ipcRenderer.invoke('desktop.daemon.stop') as Promise<DesktopDaemonStatus>,
    restartDaemon: () => ipcRenderer.invoke('desktop.daemon.restart') as Promise<DesktopDaemonStatus>,
    getAuthSession: () => ipcRenderer.invoke('desktop.authSession') as Promise<DesktopAuthResult>,
    login: (input: DesktopLoginInput) => ipcRenderer.invoke('desktop.login', input) as Promise<DesktopAuthResult>,
    logout: () => ipcRenderer.invoke('desktop.logout') as Promise<DesktopAuthResult>,
    getServerBootstrap: () => ipcRenderer.invoke('desktop.serverBootstrap') as ReturnType<DesktopApi['getServerBootstrap']>,
    getAssets: (projectID) => ipcRenderer.invoke('desktop.serverAssets', projectID) as ReturnType<DesktopApi['getAssets']>,
    getDeliveries: () => ipcRenderer.invoke('desktop.serverDeliveries') as ReturnType<DesktopApi['getDeliveries']>,
    downloadDelivery: (artifactID, fileName) => ipcRenderer.invoke('desktop.delivery.download', artifactID, fileName) as Promise<DesktopDeliveryDownloadResult>,
    chooseFiles: () => ipcRenderer.invoke('desktop.files.choose') as Promise<DesktopFileSelectionResult>,
    uploadMaterials: (projectID, fileIDs) => ipcRenderer.invoke('desktop.materials.upload', projectID, fileIDs) as Promise<DesktopMaterialUploadResult[]>,
    previewMaterial: (materialRef) => ipcRenderer.invoke('desktop.materials.preview', materialRef) as Promise<DesktopMaterialPreviewResult>,
    createServerProject: (input: DesktopCreateServerProjectInput) => ipcRenderer.invoke('desktop.serverProject.create', input) as ReturnType<DesktopApi['createServerProject']>,
    createConnectSession: (projectID: string) => ipcRenderer.invoke('desktop.connectSession.create', projectID) as ReturnType<DesktopApi['createConnectSession']>,
    getConnectSession: (sessionID: string) => ipcRenderer.invoke('desktop.connectSession.show', sessionID) as ReturnType<DesktopApi['getConnectSession']>,
    cancelConnectSession: (sessionID: string) => ipcRenderer.invoke('desktop.connectSession.cancel', sessionID) as ReturnType<DesktopApi['cancelConnectSession']>,
    getProjectEvents: (projectID: string, after: number) => ipcRenderer.invoke('desktop.projectEvents', projectID, after) as Promise<DesktopEventStreamResult>,
    publishWorkspace: (input: PublishWorkspaceInput) => ipcRenderer.invoke('desktop.publishWorkspace', input) as Promise<DesktopCommandResult>,
    retryWorkspace: (input) => ipcRenderer.invoke('desktop.retryWorkspace', input) as Promise<DesktopCommandResult>,
    getAppInfo: () => ipcRenderer.invoke('desktop.appInfo') as Promise<DesktopAppInfo>,
    onSnapshotChanged: (listener) => {
      const handler = (...args: unknown[]) => listener(args[1] as DesktopSnapshotResult);
      ipcRenderer.on('desktop.snapshotChanged', handler);
      return () => ipcRenderer.removeListener('desktop.snapshotChanged', handler);
    },
    getReviewInbox: (projectID) => ipcRenderer.invoke('desktop.reviewInbox', projectID) as ReturnType<DesktopApi['getReviewInbox']>,
    getReviewRevision: (projectID, revisionID) => ipcRenderer.invoke('desktop.reviewRevision', { projectID, revisionID }) as ReturnType<DesktopApi['getReviewRevision']>,
    addReviewComment: (projectID, input: DesktopReviewCommentInput) => ipcRenderer.invoke('desktop.reviewComment', { projectID, payload: input }) as ReturnType<DesktopApi['addReviewComment']>,
    decideReview: (projectID, revisionID, action, input: Omit<DesktopReviewDecisionInput, 'revision_id'>) => ipcRenderer.invoke('desktop.reviewDecision', { projectID, revisionID, action, payload: input }) as ReturnType<DesktopApi['decideReview']>,
  };
}
