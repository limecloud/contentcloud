#!/usr/bin/env node
/*
 * 一次性配色迁移脚本：把 web/src/styles.css 的暖绿调配色迁到
 * loopany-platform 的冷蓝灰 + Rubik 语义色体系。
 *
 * 策略：在 HSL 空间做色相旋转，保留每个颜色的明度（L）不变 ——
 * 明度关系就是原设计的层次结构，只换色相/饱和度不会动布局和可读性层级。
 *
 * 用法：node scripts/recolor.mjs [--dry]
 */
import { readFileSync, writeFileSync } from 'node:fs';

const CSS_PATH = new URL('../web/src/styles.css', import.meta.url);

// ---- 色彩空间转换 --------------------------------------------------------
function hexToRgb(hex) {
  let h = hex.slice(1);
  if (h.length === 3) h = h.split('').map((c) => c + c).join('');
  return [parseInt(h.slice(0, 2), 16), parseInt(h.slice(2, 4), 16), parseInt(h.slice(4, 6), 16)];
}

function rgbToHsl(r, g, b) {
  r /= 255; g /= 255; b /= 255;
  const max = Math.max(r, g, b), min = Math.min(r, g, b);
  const l = (max + min) / 2;
  const d = max - min;
  if (d === 0) return [0, 0, l];
  const s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
  let h;
  if (max === r) h = ((g - b) / d) % 6;
  else if (max === g) h = (b - r) / d + 2;
  else h = (r - g) / d + 4;
  h *= 60;
  if (h < 0) h += 360;
  return [h, s, l];
}

function hslToHex(h, s, l) {
  h = ((h % 360) + 360) % 360;
  s = Math.min(1, Math.max(0, s));
  l = Math.min(1, Math.max(0, l));
  const c = (1 - Math.abs(2 * l - 1)) * s;
  const x = c * (1 - Math.abs(((h / 60) % 2) - 1));
  const m = l - c / 2;
  let rgb;
  if (h < 60) rgb = [c, x, 0];
  else if (h < 120) rgb = [x, c, 0];
  else if (h < 180) rgb = [0, c, x];
  else if (h < 240) rgb = [0, x, c];
  else if (h < 300) rgb = [x, 0, c];
  else rgb = [c, 0, x];
  return '#' + rgb
    .map((v) => Math.round((v + m) * 255).toString(16).padStart(2, '0'))
    .join('');
}

// ---- 映射规则 ------------------------------------------------------------
const NEUTRAL_HUE = 228;   // 冷蓝灰
const HUE_RED = 349;       // Rubik red  #b71234
const HUE_ORANGE = 26;     // Rubik orange 收敛后的琥珀
const HUE_GREEN = 150;     // Rubik green #009b48
const HUE_BLUE = 216;      // Rubik blue  #0046ad

/*
 * 品牌/交互用途的砖红 -> 蓝。这些不是「错误」而是「可点击」，
 * 语义修正必须显式列出，不能靠色相范围自动判断（它们和错误红同色相）。
 */
const EXPLICIT = {
  '#d84b3e': '#0046ad', // --accent 品牌/交互主色
  '#b93b30': '#00379a', // --accent-dark 主按钮 hover
  '#f5efee': '#eceef4', // .auth-bg 渐变中段的暖粉 -> 冷灰
  '#fff4f2': '#eef2fb', // .relation-focus 聚焦底色 -> 蓝调
  '#f5ecea': '#eaeef7', // .run-icon 底色 -> 蓝调
};

/*
 * 语义软背景（status 胶囊、banner 底色等）。它们是被稀释过的语义色，
 * 色度天然低于中性判定阈值，若不豁免就会被当成浅灰洗掉颜色 ——
 * 而「成功绿底/错误红底」正是这套 UI 传达状态的主要手段。
 * 豁免后它们照常走色相映射，明度和饱和度不变。
 */
const SEMANTIC_SOFT = new Set([
  '#e7f5ef', // --green-soft
  '#e6f4f7', // --cyan-soft
  '#fdebea', // status-failed / blocked / rejected
  '#edf5f7', '#d5e8ec', // banner-info 底 + 描边
  '#fdeeed', // banner-error 底
  '#fff8e8', // impact-panel 琥珀底
  '#f8eeee', // evidence 危险按钮 hover
  '#dbece5', // submission 选中态绿底
  '#e5f0f6', // status-submitted / in_review
  '#eef5f8', // agent-bootstrap 进行中
  '#fdf0ef', // agent-bootstrap 错误
]);

function mapColor(hex) {
  const lower = hex.toLowerCase();
  if (EXPLICIT[lower]) return EXPLICIT[lower];

  const [r, g, b] = hexToRgb(lower);
  const [h, s, l] = rgbToHsl(r, g, b);

  // 纯黑/纯白/无彩色：保持
  if (s === 0) return lower;

  /*
   * 中性判定用 RGB 色度差（max-min），不用 HSL 饱和度：接近黑/白时
   * HSL 的 s 会被极端明度放大（#f3f7f7 只差 4/255 却算出 s=0.2），
   * 用它分类会把浅灰误判成彩色。色度差是绝对量，不受明度影响。
   */
  const chroma = Math.max(r, g, b) - Math.min(r, g, b);
  if (chroma < 26 && !SEMANTIC_SOFT.has(lower)) {
    // 中性灰：整体转冷蓝调。饱和度给温和上限，避免灰发蓝过头。
    const ns = Math.min(Math.max(s * 1.15, 0.04), 0.12);
    return hslToHex(NEUTRAL_HUE, ns, l);
  }

  // 有彩色：按色相归入 Rubik 语义色
  let target;
  if (h >= 330 || h < 22) target = HUE_RED;
  else if (h < 70) target = HUE_ORANGE;
  else if (h < 185) target = HUE_GREEN;
  else if (h < 260) target = HUE_BLUE;
  else target = HUE_BLUE;

  return hslToHex(target, s, l);
}

// ---- 执行 ----------------------------------------------------------------
const dry = process.argv.includes('--dry');
/*
 * hex 与 rgba 分开开关：hex 映射不是幂等的（中性灰的饱和度每跑一次都会
 * 被重新钳制一点），所以 hex 跑过之后只能用 --rgba-only 补做 rgba 部分。
 */
const rgbaOnly = process.argv.includes('--rgba-only');
const css = readFileSync(CSS_PATH, 'utf8');

const changes = new Map();
let out = css;

if (!rgbaOnly) {
  out = out.replace(/#[0-9a-fA-F]{6}\b|#[0-9a-fA-F]{3}\b/g, (m) => {
    const mapped = mapColor(m.toLowerCase());
    if (mapped !== m.toLowerCase()) changes.set(m.toLowerCase(), mapped);
    return mapped;
  });
}

// rgba(r,g,b,a) —— 阴影和描边大量用它，漏掉会留下暖调残影
out = out.replace(/rgba?\((\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*([^)]*)\)/g, (m, r, g, b, rest) => {
  const hex = '#' + [r, g, b].map((v) => Number(v).toString(16).padStart(2, '0')).join('');
  // 纯白/纯黑用于蒙层和高光，保持中性
  if (hex === '#ffffff' || hex === '#000000') return m;
  const mapped = mapColor(hex);
  if (mapped === hex) return m;
  const [nr, ng, nb] = hexToRgb(mapped);
  changes.set(m, `rgba(${nr},${ng},${nb}${rest})`);
  return `rgba(${nr},${ng},${nb}${rest})`;
});

const sorted = [...changes.entries()].sort();
console.log(`共映射 ${sorted.length} 个色值：`);
for (const [from, to] of sorted) console.log(`  ${from} -> ${to}`);

if (!dry) {
  writeFileSync(CSS_PATH, out);
  console.log('\n已写入 web/src/styles.css');
} else {
  console.log('\n(dry run，未写入)');
}
