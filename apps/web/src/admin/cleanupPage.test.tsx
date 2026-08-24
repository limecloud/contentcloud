import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { AdminCleanupPage, cleanupStatusLabel } from './views/AdminCleanupPage';

describe('admin cleanup diagnostics', () => {
  it('uses explicit labels for durable cleanup states', () => {
    expect(cleanupStatusLabel('pending')).toBe('待清理');
    expect(cleanupStatusLabel('cleaned')).toBe('已清理');
    expect(cleanupStatusLabel('not_found')).toBe('对象已不存在');
    expect(cleanupStatusLabel('unknown')).toBe('unknown');
  });

  it('renders the operator boundary before loading remote facts', () => {
    const markup = renderToStaticMarkup(<AdminCleanupPage />);
    expect(markup).toContain('清理诊断');
    expect(markup).toContain('不会创建任务、审批或产物事实');
  });
});
