import { expect, _electron as electron, test } from '@playwright/test';
import { resolve } from 'node:path';

test('renderer has only the narrow preload API', async () => {
  const electronPackage = await import('electron');
  const executablePath = electronPackage.default as unknown as string;
  const application = await electron.launch({ executablePath, args: [resolve('.')] });
  try {
    const page = await application.firstWindow();
    await expect(page.getByText('Content Work OS', { exact: true })).toBeVisible();
    const boundary = await page.evaluate(() => ({
      requireType: typeof (window as unknown as { require?: unknown }).require,
      processType: typeof (window as unknown as { process?: unknown }).process,
      api: Object.keys(window.contentcloudDesktop).sort(),
    }));
    expect(boundary.requireType).toBe('undefined');
    expect(boundary.processType).toBe('undefined');
    expect(boundary.api).toEqual(['addReviewComment', 'cancelConnectSession', 'chooseFiles', 'createConnectSession', 'createServerProject', 'decideReview', 'downloadDelivery', 'getAppInfo', 'getAssets', 'getAuthSession', 'getConnectSession', 'getDaemonStatus', 'getDeliveries', 'getProjectEvents', 'getReviewInbox', 'getReviewRevision', 'getServerBootstrap', 'getSnapshot', 'login', 'logout', 'onSnapshotChanged', 'previewMaterial', 'publishWorkspace', 'restartDaemon', 'retryWorkspace', 'startDaemon', 'stopDaemon', 'uploadMaterials']);
    await expect(page.getByRole('heading', { name: /本地服务未连接|内容目录/ })).toBeVisible();
  } finally {
    await application.close();
  }
});
