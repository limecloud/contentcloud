import { describe, expect, it } from 'vitest';

import { createDesktopApi, type PreloadIPC } from './api';

class FakeIPC implements PreloadIPC {
  calls: Array<{ channel: string; args: unknown[] }> = [];
  listeners = new Map<string, (...args: unknown[]) => void>();

  invoke(channel: string, ...args: unknown[]): Promise<unknown> {
    this.calls.push({ channel, args });
    return Promise.resolve({ status: 'ready' });
  }

  on(channel: string, listener: (...args: unknown[]) => void): void {
    this.listeners.set(channel, listener);
  }

  removeListener(channel: string, listener: (...args: unknown[]) => void): void {
    if (this.listeners.get(channel) === listener) this.listeners.delete(channel);
  }
}

describe('preload desktop API bridge', () => {
  it('exposes only typed channels and removes snapshot listeners', async () => {
    const ipc = new FakeIPC();
    const api = createDesktopApi(ipc);
    await api.getDaemonStatus();
    await api.startDaemon();
    await api.stopDaemon();
    await api.restartDaemon();
    await api.getServerBootstrap();
    await api.getAssets('project-1');
    await api.getDeliveries();
    await api.downloadDelivery('artifact-1', 'script.md');
    await api.chooseFiles();
    await api.uploadMaterials('project-1', ['file-1']);
    await api.previewMaterial('material-1');
    await api.createServerProject({ brand_name: 'Brand', product_name: 'Product' });
    await api.createConnectSession('project-1');
    await api.getConnectSession('session-1');
    await api.cancelConnectSession('session-1');
    await api.getProjectEvents('project-1', 0);
    await api.getReviewInbox('project-1');
    await api.getReviewRevision('project-1', 'revision-1');
    await api.addReviewComment('project-1', { revision_id: 'revision-1', body: 'comment' });
    await api.decideReview('project-1', 'revision-1', 'request-changes', { reason: 'reason' });
    expect(ipc.calls.map((call) => call.channel)).toEqual([
      'desktop.daemon.status',
      'desktop.daemon.start',
      'desktop.daemon.stop',
      'desktop.daemon.restart',
      'desktop.serverBootstrap',
      'desktop.serverAssets',
      'desktop.serverDeliveries',
      'desktop.delivery.download',
      'desktop.files.choose',
      'desktop.materials.upload',
      'desktop.materials.preview',
      'desktop.serverProject.create',
      'desktop.connectSession.create',
      'desktop.connectSession.show',
      'desktop.connectSession.cancel',
      'desktop.projectEvents',
      'desktop.reviewInbox',
      'desktop.reviewRevision',
      'desktop.reviewComment',
      'desktop.reviewDecision',
    ]);
    expect(ipc.calls[17].args[0]).toEqual({ projectID: 'project-1', revisionID: 'revision-1' });

    const events: unknown[] = [];
    const unsubscribe = api.onSnapshotChanged((value) => events.push(value));
    ipc.listeners.get('desktop.snapshotChanged')?.({}, { status: 'offline', message: 'offline' });
    expect(events).toEqual([{ status: 'offline', message: 'offline' }]);
    unsubscribe();
    expect(ipc.listeners.has('desktop.snapshotChanged')).toBe(false);
  });
});
