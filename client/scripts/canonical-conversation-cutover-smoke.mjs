import assert from 'node:assert/strict';
import { readdir, readFile, stat } from 'node:fs/promises';
import path from 'node:path';

const clientRoot = process.cwd();
const repoRoot = path.resolve(clientRoot, '..');
const productRoot = path.join(clientRoot, 'src');
const productRoots = [productRoot, path.join(repoRoot, 'internal', 'runtime'), path.join(repoRoot, 'desktop')];

const forbiddenFiles = [
  'runtime/canonicalConversationMode.ts',
  'runtime/conversation/turnProjection.ts',
  'runtime/outputReducer.ts',
  'runtime/outputSelectors.ts',
  'runtime/outputStore.ts',
  'runtime/outputStream.ts',
  'runtime/outputTypes.ts',
  'runtime/todoOutput.ts',
];

const forbiddenSymbols = [
  ['RuntimeConversationItem', /\bRuntimeConversationItem\b/],
  ['RuntimeOutputSnapshot', /\bRuntimeOutputSnapshot\b/],
  ['RuntimeOutputEvent', /\bRuntimeOutputEvent\b/],
  ['OutputStore', /\bOutputStore\b/],
  ['outputStore', /\boutputStore\b/],
  ['SessionOutput', /\bSessionOutput(?:Events)?\b/],
  ['StartSessionOutputStream', /\bStartSessionOutputStream\b/],
  ['StopSessionOutputStream', /\bStopSessionOutputStream\b/],
  ['subscribeSessionOutput', /\bsubscribeSessionOutput\b/],
  ['canonicalConversationMode', /\bcanonicalConversationMode\b/],
  ['canonicalConversationEnabled', /\bcanonicalConversationEnabled\b/],
  ['legacy conversation mode', /\bruntimeConversationModeLegacy\b|\bconversationModeLegacy\b/],
  ['shadow conversation mode', /['"]canonical_v2_shadow['"]/],
  ['conversation mode environment flag', /AGENT_BUILDER_CONVERSATION_MODE/],
  ['legacy output stream event', /agent-builder:output-stream/],
];

for (const relative of forbiddenFiles) {
  const absolute = path.join(productRoot, ...relative.split('/'));
  await assert.rejects(stat(absolute), { code: 'ENOENT' }, `${relative} must be deleted after canonical cutover`);
}

const violations = [];
for (const sourceRoot of productRoots) {
  for (const file of await sourceFiles(sourceRoot)) {
    const source = await readFile(file, 'utf8');
    for (const [label, pattern] of forbiddenSymbols) {
      if (pattern.test(source)) {
        violations.push(`${path.relative(repoRoot, file)}: ${label}`);
      }
    }
  }
}

assert.deepEqual(violations, [], `legacy conversation product symbols remain:\n${violations.join('\n')}`);

const shell = await readFile(path.join(productRoot, 'app', 'shell', 'WorkbenchShell.tsx'), 'utf8');
const adapter = await readFile(path.join(productRoot, 'runtime', 'wailsWorkbenchAdapter.ts'), 'utf8');
const workspace = await readFile(path.join(productRoot, 'features', 'workspace', 'Workspace.tsx'), 'utf8');
assert.match(shell, /createCanonicalConversationCoordinator/, 'shell must use the canonical coordinator');
assert.match(adapter, /SessionConversationSnapshotV2/, 'adapter must expose the canonical snapshot');
assert.match(adapter, /StartSessionConversationStreamV2/, 'adapter must expose the canonical stream');
assert.match(workspace, /canonicalConversationStore/, 'Workspace must render the canonical store');

console.log('canonical conversation cutover smoke passed');

async function sourceFiles(directory) {
  const files = [];
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const absolute = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      files.push(...await sourceFiles(absolute));
    } else if (/\.(?:ts|tsx|go)$/.test(entry.name) && !entry.name.endsWith('_test.go')) {
      files.push(absolute);
    }
  }
  return files;
}
