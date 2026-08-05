import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const adapter = await readFile(new URL('../src/runtime/wailsWorkbenchAdapter.ts', import.meta.url), 'utf8');
const shell = await readFile(new URL('../src/app/shell/WorkbenchShell.tsx', import.meta.url), 'utf8');
const sidebar = await readFile(new URL('../src/features/sidebar/Sidebar.tsx', import.meta.url), 'utf8');

assert.match(adapter, /SessionPage\?: \(req: \{ cursor\?: string; limit\?: number \}\)/, 'Wails bridge must expose bounded Session pages');
assert.match(adapter, /bridge\.SessionPage\(\{ cursor, limit: 50 \}\)/, 'load-more must request a fixed-size page');
assert.match(adapter, /function replaceSessionPage/, 'Session page replacement must preserve only sticky active summaries');
assert.match(adapter, /positions = new Map/, 'Session page replacement must merge sticky summaries by stable identity');
assert.match(adapter, /nextCursor: sidebarProjection\.sessionNextCursor/, 'initial sidebar projection must retain the keyset cursor');
assert.match(shell, /adapter\.loadMoreSessions/, 'Workbench shell must load Session pages explicitly');
assert.match(sidebar, /viewModel\.sessionPage\?\.hasMore/, 'Sidebar must render load-more only when the Runtime reports another page');
assert.match(sidebar, /Show older sessions/, 'Sidebar must provide explicit older-page navigation');
assert.match(sidebar, /Show newer sessions/, 'Sidebar must provide explicit newer-page navigation');
assert.match(adapter, /sessions: replaceSessionPage\(current\.sessions, incoming\)/, 'navigation must replace the cached page instead of accumulating every Session');
assert.match(sidebar, /\{ standaloneSessions, sessionsByProject \} = useMemo/, 'Sidebar must index loaded Sessions once per page generation');
assert.doesNotMatch(sidebar, /viewModel\.projects\.map[\s\S]*?viewModel\.sessions\.filter/, 'project rendering must not rescan every loaded Session');

console.log('session pagination bounds smoke passed');
