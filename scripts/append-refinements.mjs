#!/usr/bin/env node
/*
 * 把视觉重构第 4 阶段（组件精修）追加到 styles.css 末尾。
 *
 * 原文件是压缩过的单体 CSS，直接在里面改可读性很差；精修集中成一段
 * 带注释的普通 CSS 放在末尾，同特异性下后写的生效，意图也留得下来。
 *
 * 幂等：重复执行会先删掉上一次追加的块再重写。
 */
import { readFileSync, writeFileSync } from 'node:fs';

const CSS_PATH = new URL('../web/src/styles.css', import.meta.url);
const START = '/* === 设计体系精修 (redesign) === */';
const END = '/* === 设计体系精修 end === */';

const BLOCK = `
${START}

/*
 * 内容卡片统一一层极轻阴影，从冷灰底色上浮起来 —— 只给「读的容器」，
 * 控件（按钮/下拉/搜索框）保持纯描边，避免整页都在发光。
 */
.stat-grid,
.project-facts,
.gate-summary,
.brief-item,
.version-list,
.script-detail,
.public-panel,
.verify-panel,
.result-summary,
.lineage-summary,
.submission-summary,
.admin-stat-grid,
.compact-stats {
  box-shadow: var(--shadow-card);
}

/* 弹层用更强的一档，和内容层拉开距离 */
.modal,
.tenant-menu,
.project-menu {
  box-shadow: var(--shadow-pop);
}

/*
 * 蓝色（--accent）只留给可交互元素。原设计里它同时是装饰色，
 * 满页蓝字会让真正能点的东西失去信号价值。
 */
.nav-item.active,
.nav-item:hover {
  color: #fff;
}

/* 主按钮：hover 加深 + 极轻抬起，给出可点的物理感 */
.button-primary {
  box-shadow: var(--shadow-card);
  transition: background-color .15s ease, box-shadow .15s ease;
}
.button-primary:hover:not(:disabled) {
  background: var(--accent-dark);
}

/* 次级按钮描边收细，和卡片描边同一套灰 */
.button-secondary {
  transition: background-color .15s ease, border-color .15s ease;
}
.button-secondary:hover:not(:disabled) {
  background: var(--soft);
  border-color: var(--line);
}

/*
 * 表格/列表行的 hover 反馈统一到 --soft，原来散落着好几种近似灰。
 */
.project-row:hover,
.team-member-row:hover,
.brief-item:hover {
  background: var(--soft);
}

/* 焦点环统一：双层（底色留空 + 蓝环），键盘可达性一致 */
:where(button, a, input, select, textarea, [tabindex]):focus-visible {
  outline: none;
  box-shadow: 0 0 0 2px #fff, 0 0 0 4px var(--accent);
}

${END}
`;

let css = readFileSync(CSS_PATH, 'utf8');
const s = css.indexOf(START);
if (s !== -1) {
  const e = css.indexOf(END, s);
  css = css.slice(0, s).trimEnd() + (e === -1 ? '' : css.slice(e + END.length));
}
writeFileSync(CSS_PATH, css.trimEnd() + '\n' + BLOCK);
console.log('已追加设计体系精修块');
