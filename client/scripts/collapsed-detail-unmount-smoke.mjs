import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { createServer } from 'vite';

const trace = await readFile(new URL('../src/features/timeline/TraceRow.tsx', import.meta.url), 'utf8');
const tools = await readFile(new URL('../src/features/tools/ToolCallCard.tsx', import.meta.url), 'utf8');
const process = await readFile(new URL('../src/features/timeline/ProcessDisclosure.tsx', import.meta.url), 'utf8');
const hookDrawer = await readFile(new URL('../src/features/hooks/HookExecutionDetailDrawer.tsx', import.meta.url), 'utf8');
const diagnostics = await readFile(new URL('../src/features/settings/DiagnosticsSettings.tsx', import.meta.url), 'utf8');
const workspace = await readFile(new URL('../src/features/workspace/Workspace.tsx', import.meta.url), 'utf8');
const terminalPane = await readFile(new URL('../src/features/workspace/TerminalPane.tsx', import.meta.url), 'utf8');
const terminalRuntime = await readFile(new URL('../src/features/workspace/terminalRuntime.ts', import.meta.url), 'utf8');

assert.match(process, /disclosure\.open && groupedItems\.length > 0/, 'closed Process disclosure must not mount grouped items');
assert.match(trace, /\{hasBody && open \? \(/, 'closed TraceRow must not mount its detail body');
assert.match(trace, /\{open \? \([\s\S]*styles\.rowExpandInner/, 'closed InlineExpandable must not mount its detail body');
assert.match(tools, /\{hasDetails && open \? \(/, 'closed QuietToolRow must not create ToolDetails');
assert.equal(tools.match(/destroyOnHidden/g)?.length, 4, 'all three Collapse instances and the full-content Drawer must destroy hidden content');
assert.match(tools, /<Drawer destroyOnHidden open=\{Boolean\(fullContent\)\}/, 'full content must use one destroy-on-close Drawer per mounted detail');
assert.match(tools, /const close = useCallback\(\(\) => \{[\s\S]*setFullContent\(undefined\)/, 'closing full content must release its text state');
assert.match(tools, /generationRef\.current === generation/, 'late Object reads must not repopulate a replaced or closed Drawer');
assert.equal(tools.match(/useState<\{ title: string; text: string; loading: boolean \}>/g)?.length, 1, 'only the singleton Tool Drawer may own full-content state');
assert.match(workspace, /<ToolDetailDrawerProvider key=\{activeSessionID\}/, 'Session switching must synchronously destroy the singleton Tool Drawer');
assert.match(hookDrawer, /<Drawer destroyOnHidden/, 'hook detail DOM must be destroyed when closed');
assert.match(hookDrawer, /if \(!visible\) releaseDetail\(\)/, 'hook detail state must be released after close');
assert.match(diagnostics, /<Drawer destroyOnHidden/, 'diagnostic detail DOM must be destroyed when closed');
assert.match(terminalPane, /return \(\) => disposeTerminalRuntime\(terminalID\)/, 'unmounting the visible terminal pane must release its renderer and detailed stream');
assert.match(terminalRuntime, /runtime\.unsubscribe\?\.\(\)/, 'disposing a terminal renderer must stop its Wails subscription');
assert.match(terminalRuntime, /if \(runtime\.disposed\) \{\s*dispose\(\);\s*return;/, 'a late terminal subscription must close instead of attaching to a disposed renderer');
assert.match(terminalRuntime, /runtime\.xterm\.dispose\(\)/, 'disposing a terminal renderer must release xterm state');

const vite = await createServer({
  root: fileURLToPath(new URL('..', import.meta.url)),
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
});
try {
  const { TraceRow, InlineExpandable } = await vite.ssrLoadModule('/src/features/timeline/TraceRow.tsx');
  const { ToolCallCard } = await vite.ssrLoadModule('/src/features/tools/ToolCallCard.tsx');
  const marker = React.createElement('span', null, 'DETAIL_DOM_MARKER');
  const traceHTML = renderToStaticMarkup(React.createElement(TraceRow, { expandable: true, title: 'row' }, marker));
  const inlineHTML = renderToStaticMarkup(React.createElement(InlineExpandable, { summary: 'summary' }, marker));
  assert.doesNotMatch(traceHTML, /DETAIL_DOM_MARKER/, 'closed TraceRow rendered detail DOM');
  assert.doesNotMatch(inlineHTML, /DETAIL_DOM_MARKER/, 'closed InlineExpandable rendered detail DOM');

  const secret = 'FULL_TOOL_DETAIL_MUST_NOT_MOUNT';
  const normalToolHTML = renderToStaticMarkup(React.createElement(ToolCallCard, {
    toolCall: { id: 'tool-normal', name: 'shell', status: 'failed', kind: 'shell', inputRef: 'runtime://objects/full-input', outputSummary: secret },
  }));
  const quietToolHTML = renderToStaticMarkup(React.createElement(ToolCallCard, {
    toolCall: { id: 'tool-quiet', name: 'read', status: 'completed', kind: 'file_read', quiet: true, outputSummary: secret },
  }));
  const collapsedGroupHTML = renderToStaticMarkup(React.createElement(ToolCallCard, {
    toolCalls: Array.from({ length: 100 }, (_, index) => ({
      id: `tool-group-${index}`,
      name: 'read',
      status: 'completed',
      kind: 'file_read',
      quiet: true,
      outputSummary: `${secret}-${index}`,
    })),
  }));
  assert.doesNotMatch(normalToolHTML, new RegExp(secret), 'closed Collapse rendered ToolDetails output');
  assert.doesNotMatch(normalToolHTML, /Read full input/, 'closed Collapse mounted the lazy input-ref action');
  assert.doesNotMatch(quietToolHTML, new RegExp(secret), 'closed QuietToolRow rendered ToolDetails output');
  assert.doesNotMatch(collapsedGroupHTML, new RegExp(secret), 'collapsed 100-tool group rendered row details');
  assert.doesNotMatch(collapsedGroupHTML, /<pre\b/, 'collapsed 100-tool group rendered preview DOM');
} finally {
  await vite.close();
}

console.log('collapsed detail unmount smoke passed');
