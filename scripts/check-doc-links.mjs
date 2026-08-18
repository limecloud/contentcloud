import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { dirname, extname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(fileURLToPath(new URL('..', import.meta.url)));
const ignoredDirectories = new Set(['.vite', 'dist', 'node_modules', 'out']);
const markdownFiles = [];

function walk(directory) {
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    if (entry.isDirectory() && ignoredDirectories.has(entry.name)) continue;
    const path = join(directory, entry.name);
    if (entry.isDirectory()) walk(path);
    else if (extname(entry.name) === '.md') markdownFiles.push(path);
  }
}

function destination(raw) {
  const value = raw.trim();
  if (value.startsWith('<')) {
    const end = value.indexOf('>');
    return end > 0 ? value.slice(1, end) : value;
  }
  return value.split(/\s+["']/, 1)[0];
}

walk(join(root, 'docs'));
markdownFiles.push(join(root, 'README.md'));
const failures = [];
for (const path of markdownFiles) {
  const body = readFileSync(path, 'utf8');
  for (const match of body.matchAll(/!?\[[^\]]*\]\(([^)]+)\)/g)) {
    let target = destination(match[1]);
    if (/^(?:https?:|mailto:|data:|#|\/)/.test(target)) continue;
    target = target.split('#', 1)[0].split('?', 1)[0];
    if (!target) continue;
    try {
      target = decodeURIComponent(target);
    } catch {
      failures.push(`${relative(root, path)}: malformed link ${match[1]}`);
      continue;
    }
    if (!existsSync(resolve(dirname(path), target))) {
      failures.push(`${relative(root, path)}: missing link target ${match[1]}`);
    }
  }
}

if (failures.length) {
  console.error(`Documentation link checks failed:\n${failures.map((value) => `- ${value}`).join('\n')}`);
  process.exit(1);
}
console.log(`Documentation links passed (${markdownFiles.length} Markdown files).`);
