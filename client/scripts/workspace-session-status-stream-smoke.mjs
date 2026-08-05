import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const shell = await readFile(new URL('../src/app/shell/WorkbenchShell.tsx', import.meta.url), 'utf8');
const refresh = await readFile(new URL('../src/runtime/runtimeEventRefresh.ts', import.meta.url), 'utf8');
const adapter = await readFile(new URL('../src/runtime/wailsWorkbenchAdapter.ts', import.meta.url), 'utf8');
const types = await readFile(new URL('../src/runtime/workbenchTypes.ts', import.meta.url), 'utf8');

assert.match(shell, /applyRuntimeSessionStatusEvent\(current, event\)/, 'the Workspace event stream must update lightweight Session status directly');
assert.doesNotMatch(shell, /refreshUntilIdle|busyIntervalMs|hasBusySession/, 'busy Sessions must not create a polling refresh loop');
assert.match(refresh, /const turnStartedEvents = new Set/, 'turn start events must enter the active status index');
assert.match(refresh, /const turnFinishedEvents = new Set/, 'terminal turn events must leave the active status index');
assert.match(refresh, /session\.activeTurnId !== turnID/, 'late terminal events must not clear a newer Turn');
assert.doesNotMatch(refresh, /event\.payload/, 'the view model must use bounded event envelope fields only');
assert.match(types, /activeSessionStatuses\?: ActiveSessionStatusViewModel\[\]/, 'React must retain only the reconstructable active Session summary index');
assert.match(adapter, /status\?\.activeSessions/, 'the active Session index must come from the Runtime status DTO');
assert.match(adapter, /\.slice\(0, 500\)/, 'the client projection must preserve the Runtime 500-entry bound');
assert.doesNotMatch(adapter, /bridge\.Turns\?\.\('active'\)/, 'Workbench hydration must not fetch full active Turn DTOs');

console.log('Workspace Session status stream smoke passed');
