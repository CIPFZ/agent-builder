import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const adapterPath = resolve(repoRoot, 'client', 'src', 'runtime', 'wailsWorkbenchAdapter.ts');
const typesPath = resolve(repoRoot, 'client', 'src', 'runtime', 'workbenchTypes.ts');
const toolCardPath = resolve(repoRoot, 'client', 'src', 'features', 'tools', 'ToolCallCard.tsx');
const shellPath = resolve(repoRoot, 'client', 'src', 'app', 'shell', 'WorkbenchShell.tsx');

const [adapterSource, typesSource, toolCardSource, shellSource] = await Promise.all([
  readFile(adapterPath, 'utf8'),
  readFile(typesPath, 'utf8'),
  readFile(toolCardPath, 'utf8'),
  readFile(shellPath, 'utf8'),
]);

assert.match(typesSource, /ConversationTimelineKind = .*'turn_terminal'/s);
assert.match(typesSource, /sequence\?: number;/);
assert.doesNotMatch(typesSource, /export interface ToolCallViewModel[\s\S]*?\n}\n[\s\S]*?stdout\?: string/);
assert.doesNotMatch(typesSource, /export interface ToolCallViewModel[\s\S]*?\n}\n[\s\S]*?stderr\?: string/);

assert.match(adapterSource, /function buildRuntimeTimelineOrder/);
assert.match(adapterSource, /node\.kind === 'assistant_step'/);
assert.match(adapterSource, /node\.kind === 'turn_terminal'/);
assert.match(adapterSource, /sequence: runtimeOrder\.sequenceByToolCallID\.get\(toolCall\.id\)/);
assert.match(adapterSource, /sequence: runtimeOrder\.sequenceByPermissionID\.get\(permission\.id\)/);
assert.match(adapterSource, /summary: missingFinal[\s\S]*?Turn ended without a final assistant message\./);
assert.match(adapterSource, /function mapToolCall\(toolCall: RuntimeToolCallDTO\): ToolCallViewModel/);
assert.doesNotMatch(adapterSource, /toolCall:\s*{\s*\.\.\.toolCall/s);
assert.doesNotMatch(adapterSource, /stdout:\s*toolCall\.stdout/);
assert.doesNotMatch(adapterSource, /stderr:\s*toolCall\.stderr/);
assert.match(adapterSource, /if \(typeof left\.sequence === 'number' && typeof right\.sequence === 'number'/);

assert.doesNotMatch(toolCardSource, /toolCall\.stdout/);
assert.doesNotMatch(toolCardSource, /toolCall\.stderr/);
assert.doesNotMatch(toolCardSource, /extractWrappedOutput/);

assert.match(shellSource, /id: userID,[\s\S]*status: 'success'/);
assert.match(shellSource, /const nextViewModel = await adapter\.sendPrompt\(optimisticViewModel, prompt\)/);
assert.match(adapterSource, /if \(hasRuntimeActivity && isOptimisticTimelineItem\(item\)\)/);
assert.match(adapterSource, /!isOptimisticConversationMessage\(message\)/);
assert.match(adapterSource, /runtimeActionRefreshTargets\(response\)/);
assert.doesNotMatch(adapterSource, /response\.action\.(?:source|reason|evidence|payload)/);

console.log('phase07 workbench runtime rendering smoke passed');
