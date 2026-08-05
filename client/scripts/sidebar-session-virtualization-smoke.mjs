import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const sidebar = await readFile(new URL('../src/features/sidebar/Sidebar.tsx', import.meta.url), 'utf8');
const styles = await readFile(new URL('../src/features/sidebar/Sidebar.module.css', import.meta.url), 'utf8');

function numericConstant(name) {
  const match = sidebar.match(new RegExp(`const ${name} = (\\d+);`));
  assert.ok(match, `${name} must remain an explicit audited bound`);
  return Number(match[1]);
}

const rowHeight = numericConstant('sidebarSessionRowHeight');
const viewportHeight = numericConstant('sidebarSessionViewportHeight');
const overscan = numericConstant('sidebarSessionOverscan');
const totalMountedRowBudget = numericConstant('sidebarSessionMountedRowBudget');
const maximumMountedRows = Math.ceil(viewportHeight / rowHeight) + overscan * 2;

assert.ok(maximumMountedRows < 150, `one Session list can mount at most ${maximumMountedRows} rows`);
assert.ok(totalMountedRowBudget < 150, `all expanded Session lists share a ${totalMountedRowBudget}-row budget`);
assert.match(sidebar, /sessions\.slice\(start, end\)\.map/, 'Session rows must be sliced before React mounts them');
assert.match(sidebar, /data-windowed-session-list="true"/, 'windowed Session lists need an observable DOM boundary');
assert.match(sidebar, /<WindowedSessionList[\s\S]*?sessions=\{projectSessions\}/, 'project Sessions must use the shared window');
assert.match(sidebar, /<WindowedSessionList[\s\S]*?sessions=\{standaloneSessions\}/, 'standalone Sessions must use the shared window');
assert.match(sidebar, /mountedRowsPerSessionList = Math\.max\(1, Math\.floor\(sidebarSessionMountedRowBudget/, 'expanded Session lists must divide one global DOM budget');
assert.match(styles, /\.windowedSessionViewport[\s\S]*?overflow-y:\s*auto/, 'the window viewport must own bounded scrolling');

function mountedRange(total, scrollTop) {
  const height = Math.min(viewportHeight, total * rowHeight);
  const visible = Math.ceil(height / rowHeight);
  const firstVisible = Math.floor(scrollTop / rowHeight);
  const start = Math.max(0, Math.min(firstVisible - overscan, total - visible));
  const end = Math.min(total, firstVisible + visible + overscan);
  return end - start;
}

for (const listCount of [1, 2, 10, 100]) {
  const perListBudget = Math.max(1, Math.floor(totalMountedRowBudget / listCount));
  const mountedRows = Math.min(maximumMountedRows, perListBudget) * listCount;
  assert.ok(mountedRows <= totalMountedRowBudget, `${listCount} expanded lists mount ${mountedRows} rows`);
}

for (const total of [100, 1_000, 10_000]) {
  const maximumScroll = Math.max(0, total * rowHeight - viewportHeight);
  for (const scrollTop of [0, Math.floor(maximumScroll / 2), maximumScroll]) {
    assert.ok(
      mountedRange(total, scrollTop) <= maximumMountedRows,
      `${total} loaded Sessions exceeded the mounted-row budget at scrollTop=${scrollTop}`,
    );
  }
}

console.log(`sidebar Session virtualization smoke passed; maximum mounted rows per list=${maximumMountedRows}`);
