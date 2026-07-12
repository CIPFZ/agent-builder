import assert from 'node:assert/strict';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import ts from 'typescript';

const root = process.cwd();
const tempDir = path.join(root, 'node_modules', '.tmp', 'markdown-message-smoke');
await mkdir(tempDir, { recursive: true });

const sourcePath = path.join(root, 'src', 'features', 'markdown', 'MarkdownMessage.tsx');
const source = await readFile(sourcePath, 'utf8');
const stylesSource = await readFile(path.join(root, 'src', 'features', 'markdown', 'MarkdownMessage.module.css'), 'utf8');
const shimmed = source.replace(
  "import styles from './MarkdownMessage.module.css';",
  "const styles = new Proxy({}, { get: (_target, key) => String(key) });",
);
const transpiled = ts.transpileModule(shimmed, {
  compilerOptions: {
    target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.ES2022,
    jsx: ts.JsxEmit.ReactJSX,
    importsNotUsedAsValues: ts.ImportsNotUsedAsValues.Remove,
  },
}).outputText;
const outputPath = path.join(tempDir, 'MarkdownMessage.mjs');
await writeFile(outputPath, transpiled);

const { MarkdownMessage } = await import(pathToFileURL(outputPath).href);
const markdown = [
  '## Report',
  '',
  '- **bold** item',
  '',
  '| Name | Value |',
  '| --- | ---: |',
  '| alpha | 42 |',
  '',
  '```ts',
  'const value = 42;',
  '```',
  '',
  '[OpenAI](https://openai.com)',
  '',
  '<script>alert("x")</script>',
].join('\n');
const html = renderToStaticMarkup(React.createElement(MarkdownMessage, { content: markdown, role: 'assistant' }));

assert.match(html, /<h2>Report<\/h2>/, 'heading is rendered as markdown');
assert.match(html, /<ul>/, 'list is rendered as markdown');
assert.match(html, /<strong>bold<\/strong>/, 'strong text is rendered as markdown');
assert.match(html, /<div class="markdownTableWrap"><table>/, 'GFM table is rendered in a scroll wrapper');
assert.match(html, /<th>Name<\/th>/, 'table header is rendered');
assert.match(html, /<td[^>]*>42<\/td>/, 'table cell is rendered');
assert.match(html, /<pre class="markdownPre"><code class="markdownCode language-ts" data-language="ts">const value = 42;\n<\/code><\/pre>/, 'fenced code block is rendered');
assert.match(html, /target="_blank"/, 'links open outside the webview');
assert(!html.includes('<script>alert'), 'raw HTML is escaped instead of executed');
assert(!html.includes('| --- |'), 'markdown table source is not displayed as raw text');
assert.doesNotMatch(stylesSource, /\.markdownTableWrap table\s*\{[\s\S]*?min-width:\s*max-content/, 'tables cannot expand to the max-content width of long cells');
assert.match(stylesSource, /\.markdownTableWrap td \.markdownCode[\s\S]*?overflow-wrap:\s*anywhere/, 'long inline code wraps within table cells');

console.log('markdown message smoke passed');
