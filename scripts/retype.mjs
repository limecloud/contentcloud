#!/usr/bin/env node
/*
 * 一次性字号迁移：把 21 档无序字号收敛成 5 档 token，并把正文级文字
 * 从 7-10px 抬到可读区间。
 *
 * 大于 13px 的一律不动 —— 那些是标题，本来就够大；问题只在小字一端。
 * 抬升幅度随字号递减（8px +37%，12px +8%），既救回最小的一档，
 * 又不至于把整版面撑开。
 *
 * 用法：node scripts/retype.mjs [--dry]
 */
import { readFileSync, writeFileSync } from 'node:fs';

const CSS_PATH = new URL('../web/src/styles.css', import.meta.url);

// 原字号 -> token 名
const SCALE = {
  7: '--fs-micro',    // 11px
  8: '--fs-micro',
  9: '--fs-caption',  // 11.5px
  10: '--fs-label',   // 12px
  11: '--fs-meta',    // 12.5px
  12: '--fs-body',    // 13px
  13: '--fs-body',
};

const dry = process.argv.includes('--dry');
const css = readFileSync(CSS_PATH, 'utf8');

const counts = new Map();
const out = css.replace(/font-size:(\d+(?:\.\d+)?)px/g, (m, num) => {
  const token = SCALE[Number(num)];
  if (!token) return m; // >13px 的标题保持原样
  counts.set(num, (counts.get(num) || 0) + 1);
  return `font-size:var(${token})`;
});

console.log('字号迁移统计：');
for (const [px, n] of [...counts.entries()].sort((a, b) => a[0] - b[0])) {
  console.log(`  ${px}px × ${n}  ->  var(${SCALE[Number(px)]})`);
}
const remaining = [...out.matchAll(/font-size:(\d+(?:\.\d+)?)px/g)].map((m) => Number(m[1]));
console.log(`\n保持原样的字号（标题层）：${[...new Set(remaining)].sort((a, b) => a - b).join(', ')}`);

if (!dry) {
  writeFileSync(CSS_PATH, out);
  console.log('\n已写入 web/src/styles.css');
} else {
  console.log('\n(dry run，未写入)');
}
