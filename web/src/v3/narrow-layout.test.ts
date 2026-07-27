import { describe, expect, it } from 'vitest';
// @ts-expect-error Vitest 在 Node 中运行；Web 应用的类型边界不引入 Node globals。
import { readFileSync } from 'node:fs';

const css = readFileSync(new URL('./narrow-layout.css', import.meta.url), 'utf8').replace(/\s+/g, ' ');

describe('V4 narrow Browser layout contract', () => {
  it('prioritizes next action before metrics and content in a narrow overview', () => {
    expect(css).toContain('.v3-overview-layout { grid-template-areas: "next" "metrics" "flow" "revisions"; }');
  });

  it('prioritizes alerts and next action in narrow domain views', () => {
    expect(css).toContain('.v3-domain-layout { grid-template-areas: "next" "metrics" "revisions" "snapshots"; }');
    expect(css).toContain('.v3-domain-layout.has-alerts { grid-template-areas: "alerts" "next" "metrics" "revisions" "snapshots"; }');
  });

  it('removes core horizontal scrolling from the project flow', () => {
    expect(css).toContain('.v3-flow-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); overflow: visible; }');
    expect(css).toContain('.v3-flow-grid small { white-space: normal; overflow-wrap: anywhere; }');
  });

  it('reserves the smallest toolbar width for project context', () => {
    expect(css).toContain('.topbar .new-project { display: none; }');
    expect(css).toContain('.project-select-wrap { flex: 1; min-width: 0; }');
  });
});
