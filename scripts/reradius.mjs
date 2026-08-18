#!/usr/bin/env node
/*
 * 一次性圆角迁移：8 档混用的圆角收敛成 4 档 token，向 loopany 的
 * control(10) / card(16) 尺度靠拢。
 *
 * 按现有用途分组，而不是按数值均分：
 *   5px  = 控件（input/select/icon-button）
 *   6-7px = 按钮、头像、小容器 —— 这一档是主力，对应 loopany 的 control
 *   8-9px = 卡片、面板、模态
 *   14-16px = auth 大卡片，对应 sheet
 *
 * 用法：node scripts/reradius.mjs [--dry]
 */
import { readFileSync, writeFileSync } from 'node:fs';

const CSS_PATH = new URL('../apps/web/src/styles.css', import.meta.url);

const SCALE = {
  2: '--r-xs',    // 6px  装饰小块（empty-mark / severity 条）
  4: '--r-sm',    // 8px  行内小按钮、代码块
  5: '--r-sm',
  6: '--r-ctl',   // 10px 控件与按钮（主力档）
  7: '--r-ctl',
  8: '--r-card',  // 12px 卡片 / 面板 / 模态
  9: '--r-card',
  10: '--r-ctl',  // 已是目标值，一并纳入 token 以免留下裸数值
  12: '--r-card',
  14: '--r-sheet', // 16px auth 大卡片
  16: '--r-sheet',
};

const dry = process.argv.includes('--dry');
const css = readFileSync(CSS_PATH, 'utf8');

const counts = new Map();
const out = css.replace(/border-radius:(\d+(?:\.\d+)?)px/g, (m, num) => {
  const token = SCALE[Number(num)];
  if (!token) return m;
  counts.set(num, (counts.get(num) || 0) + 1);
  return `border-radius:var(${token})`;
});

console.log('圆角迁移统计：');
for (const [px, n] of [...counts.entries()].sort((a, b) => a[0] - b[0])) {
  console.log(`  ${px}px × ${n}  ->  var(${SCALE[Number(px)]})`);
}
const left = [...out.matchAll(/border-radius:([^;}]+)/g)]
  .map((m) => m[1]).filter((v) => !v.startsWith('var'));
console.log(`\n未迁移（胶囊/百分比等）：${[...new Set(left)].join(', ') || '无'}`);

if (!dry) {
  writeFileSync(CSS_PATH, out);
  console.log('\n已写入 apps/web/src/styles.css');
} else {
  console.log('\n(dry run，未写入)');
}
