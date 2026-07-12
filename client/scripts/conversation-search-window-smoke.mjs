import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const adapter = await readFile(new URL('../src/runtime/wailsWorkbenchAdapter.ts', import.meta.url), 'utf8');
const coordinator = await readFile(new URL('../src/runtime/canonicalConversationCoordinator.ts', import.meta.url), 'utf8');
const workspace = await readFile(new URL('../src/features/workspace/Workspace.tsx', import.meta.url), 'utf8');

assert.match(adapter, /SearchSessionConversationV2/, 'conversation search uses the Runtime bridge');
assert.match(coordinator, /around: turnId/, 'search navigation requests a target Turn window');
assert.match(workspace, /搜索当前对话/, 'active Session exposes conversation search');
assert.match(workspace, /scrollIntoView\(\{ block: 'center' \}\)/, 'selected result is positioned in the Timeline');
assert.doesNotMatch(adapter, /fetch\(|XMLHttpRequest|axios/, 'search cannot add a browser transport fallback');

console.log('conversation search window smoke passed');
