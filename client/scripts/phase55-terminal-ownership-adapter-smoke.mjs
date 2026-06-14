import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

const repoRoot = resolve(import.meta.dirname, '..', '..');
const adapterPath = resolve(repoRoot, 'client', 'src', 'runtime', 'wailsWorkbenchAdapter.ts');
const typesPath = resolve(repoRoot, 'client', 'src', 'runtime', 'workbenchTypes.ts');
const workspacePath = resolve(repoRoot, 'client', 'src', 'features', 'workspace', 'Workspace.tsx');

const adapter = readFileSync(adapterPath, 'utf8');
const types = readFileSync(typesPath, 'utf8');
const workspace = readFileSync(workspacePath, 'utf8');

assert.match(types, /listSessionTerminals:\s*\(sessionID: string\) => Promise<TerminalViewModel\[\]>/);
assert.match(types, /createTerminal:\s*\(request: \{ sessionId: string;/);
assert.match(types, /sessionId: string;/);
assert.match(types, /projectId\?: string;/);
assert.match(types, /initialCwd\?: string;/);

assert.match(adapter, /SessionTerminals\?: \(sessionID: string\) => Promise<RuntimeSessionTerminalsResponseDTO>/);
assert.match(adapter, /\/v1\/sessions\/\$\{encodeURIComponent\(sessionID\)\}\/terminals/);
assert.match(adapter, /async listSessionTerminals\(sessionID\)/);
assert.match(adapter, /mapSessionTerminals\(await bridge\.SessionTerminals\(sessionID\)\)/);
assert.match(adapter, /mapSessionTerminals\(await httpBridge\.SessionTerminals\(sessionID\)\)/);
assert.doesNotMatch(adapter, /action\.[A-Za-z0-9_]*terminal/i);

assert.match(workspace, /onSessionTerminalsList: \(sessionID: string\) => Promise<TerminalViewModel\[\]>/);
assert.match(workspace, /onTerminalCreate: \(request: \{ sessionId: string;/);
assert.match(workspace, /onSessionTerminalsList\(activeSessionID\)/);
assert.match(workspace, /terminal\.sessionId === activeSessionID/);
assert.match(workspace, /onTerminalCreate\(\{ sessionId: activeSession\.id, columns: 100, rows: 24 \}\)/);
assert.match(workspace, /await onTerminalDelete\(closingTerminalID\)/);
assert.match(workspace, /await refreshSessionTerminals\(activeSession\.id\)/);
assert.doesNotMatch(workspace, /onTerminalCreate\(\{ cwd: viewModel\.currentProject\.path/);

console.log('phase55 terminal ownership adapter smoke passed');
