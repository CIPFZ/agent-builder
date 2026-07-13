import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const adapter = await readFile(new URL('../src/runtime/wailsWorkbenchAdapter.ts', import.meta.url), 'utf8');
const coordinator = await readFile(new URL('../src/runtime/canonicalConversationCoordinator.ts', import.meta.url), 'utf8');
const workspace = await readFile(new URL('../src/features/workspace/Workspace.tsx', import.meta.url), 'utf8');

assert.match(adapter, /SearchSessionConversationV2/, 'conversation search uses the Runtime bridge');
assert.match(coordinator, /around: turnId/, 'search navigation requests a target Turn window');
assert.doesNotMatch(workspace, /搜索当前对话|SearchOutlined/, 'workspace header does not duplicate the global search entry');
assert.match(workspace, /const sessionTitle = activeSession\?\.title \?\? ''/, 'drafts do not borrow the project name as a Session title');
assert.match(workspace, /\{activeSession \? \([\s\S]*?sessionTitleWrap[\s\S]*?\) : <div \/>\}/, 'draft header keeps the Session title area empty until a Session exists');
assert.doesNotMatch(adapter, /fetch\(|XMLHttpRequest|axios/, 'search cannot add a browser transport fallback');

console.log('conversation search window smoke passed');
