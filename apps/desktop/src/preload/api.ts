import type {
  DesktopApi,
  DesktopAppInfo,
  DesktopCommandResult,
  DesktopReviewCommentInput,
  DesktopReviewDecisionInput,
  DesktopSnapshotResult,
  PublishWorkspaceInput,
} from '../shared/contracts';

export interface PreloadIPC {
  invoke(channel: string, ...args: unknown[]): Promise<unknown>;
  on(channel: string, listener: (...args: unknown[]) => void): void;
  removeListener(channel: string, listener: (...args: unknown[]) => void): void;
}

export function createDesktopApi(ipcRenderer: PreloadIPC): DesktopApi {
  return {
    getSnapshot: () => ipcRenderer.invoke('desktop.snapshot') as Promise<DesktopSnapshotResult>,
    publishWorkspace: (input: PublishWorkspaceInput) => ipcRenderer.invoke('desktop.publishWorkspace', input) as Promise<DesktopCommandResult>,
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
