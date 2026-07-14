import { readFile, readdir } from 'node:fs/promises';
import { extname, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const clientRoot = resolve(fileURLToPath(new URL('..', import.meta.url)));
const sourceRoot = resolve(clientRoot, 'src');
const allowedFiles = new Set([
  'theme/themes/default/light.ts',
  'theme/themes/default/dark.ts',
  'features/workspace/terminalRuntime.ts',
]);
const sourceExtensions = new Set(['.css', '.ts', '.tsx']);
const colorPattern = /#[0-9a-fA-F]{3,8}\b|rgba?\s*\(/g;

const violations = [];
for (const file of await walk(sourceRoot)) {
  if (!sourceExtensions.has(extname(file))) continue;
  const localPath = relative(sourceRoot, file).replaceAll('\\', '/');
  if (allowedFiles.has(localPath)) continue;
  const lines = (await readFile(file, 'utf8')).split(/\r?\n/);
  lines.forEach((line, index) => {
    if (colorPattern.test(line)) violations.push(`${localPath}:${index + 1}: ${line.trim()}`);
    colorPattern.lastIndex = 0;
  });
}

if (violations.length > 0) {
  console.error('Hardcoded theme colors are not allowed outside theme definitions and the terminal ANSI palette:');
  console.error(violations.join('\n'));
  process.exit(1);
}

console.log('Theme color guard passed.');

async function walk(directory) {
  const files = [];
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const path = resolve(directory, entry.name);
    if (entry.isDirectory()) files.push(...await walk(path));
    else files.push(path);
  }
  return files;
}
