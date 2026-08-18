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
    await api.getReviewInbox('project-1');
    await api.getReviewRevision('project-1', 'revision-1');
    await api.addReviewComment('project-1', { revision_id: 'revision-1', body: 'comment' });
    await api.decideReview('project-1', 'revision-1', 'request-changes', { reason: 'reason' });
    expect(ipc.calls.map((call) => call.channel)).toEqual([
      'desktop.reviewInbox',
      'desktop.reviewRevision',
      'desktop.reviewComment',
      'desktop.reviewDecision',
    ]);
    expect(ipc.calls[1].args[0]).toEqual({ projectID: 'project-1', revisionID: 'revision-1' });

    const events: unknown[] = [];
    const unsubscribe = api.onSnapshotChanged((value) => events.push(value));
    ipc.listeners.get('desktop.snapshotChanged')?.({}, { status: 'offline', message: 'offline' });
    expect(events).toEqual([{ status: 'offline', message: 'offline' }]);
    unsubscribe();
    expect(ipc.listeners.has('desktop.snapshotChanged')).toBe(false);
  });
});
