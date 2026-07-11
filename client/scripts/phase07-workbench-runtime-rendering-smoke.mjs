import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const adapterPath = resolve(repoRoot, 'client', 'src', 'runtime', 'wailsWorkbenchAdapter.ts');
const typesPath = resolve(repoRoot, 'client', 'src', 'runtime', 'workbenchTypes.ts');
const toolCardPath = resolve(repoRoot, 'client', 'src', 'features', 'tools', 'ToolCallCard.tsx');
const shellPath = resolve(repoRoot, 'client', 'src', 'app', 'shell', 'WorkbenchShell.tsx');
const turnProjectionPath = resolve(repoRoot, 'client', 'src', 'runtime', 'conversation', 'turnProjection.ts');
const timelinePath = resolve(repoRoot, 'client', 'src', 'features', 'timeline', 'Timeline.tsx');
const disclosurePath = resolve(repoRoot, 'client', 'src', 'features', 'timeline', 'ProcessDisclosure.tsx');
const narrationPath = resolve(repoRoot, 'client', 'src', 'features', 'timeline', 'ProcessNarration.tsx');
const timelineStylePath = resolve(repoRoot, 'client', 'src', 'features', 'timeline', 'Timeline.module.css');
const toolStylePath = resolve(repoRoot, 'client', 'src', 'features', 'tools', 'ToolCallCard.module.css');
const workspacePath = resolve(repoRoot, 'client', 'src', 'features', 'workspace', 'Workspace.tsx');
const traceRowPath = resolve(repoRoot, 'client', 'src', 'features', 'timeline', 'TraceRow.tsx');
const traceRowStylePath = resolve(repoRoot, 'client', 'src', 'features', 'timeline', 'TraceRow.module.css');

const [adapterSource, typesSource, toolCardSource, shellSource, turnProjectionSource, timelineSource, disclosureSource, narrationSource, timelineStyleSource, toolStyleSource, workspaceSource, traceRowSource, traceRowStyleSource] = await Promise.all([
  readFile(adapterPath, 'utf8'),
  readFile(typesPath, 'utf8'),
  readFile(toolCardPath, 'utf8'),
  readFile(shellPath, 'utf8'),
  readFile(turnProjectionPath, 'utf8'),
  readFile(timelinePath, 'utf8'),
  readFile(disclosurePath, 'utf8'),
  readFile(narrationPath, 'utf8'),
  readFile(timelineStylePath, 'utf8'),
  readFile(toolStylePath, 'utf8'),
  readFile(workspacePath, 'utf8'),
  readFile(traceRowPath, 'utf8'),
  readFile(traceRowStylePath, 'utf8'),
]);

assert.match(typesSource, /export type ConversationTimelineKind =/);
assert.match(typesSource, /\| 'turn_terminal'/);
assert.match(typesSource, /sequence\?: number;/);
assert.doesNotMatch(typesSource, /export interface ToolCallViewModel[\s\S]*?\n}\n[\s\S]*?stdout\?: string/);
assert.doesNotMatch(typesSource, /export interface ToolCallViewModel[\s\S]*?\n}\n[\s\S]*?stderr\?: string/);

assert.match(adapterSource, /function mapToolCall\(toolCall: RuntimeToolCallDTO\): ToolCallViewModel/);
assert.doesNotMatch(adapterSource, /toolCall:\s*{\s*\.\.\.toolCall/s);
assert.doesNotMatch(adapterSource, /stdout:\s*toolCall\.stdout/);
assert.doesNotMatch(adapterSource, /stderr:\s*toolCall\.stderr/);
assert.doesNotMatch(adapterSource, /buildRuntimeTimelineOrder|mergeNormalizedTimeline|mapNormalizedInputTimeline/);

assert.match(turnProjectionSource, /Object\.values\(store\.turnsById\)/);
assert.match(turnProjectionSource, /turn\.userMessageId/);
assert.match(turnProjectionSource, /turn\.latestAssistantMessageId/);
assert.match(turnProjectionSource, /item\.phase === 'final'/);
assert.match(timelineSource, /<ProcessDisclosure/);
assert.match(timelineSource, /<TimelineMessage/);
assert.match(disclosureSource, /shouldAutoOpenProcess/);
assert.doesNotMatch(disclosureSource, /failedCount: props\.exploration/);
assert.match(disclosureSource, /data-testid="process-stream"/);
assert.match(narrationSource, /data-testid="process-narration"/);
assert.match(narrationSource, /<MarkdownMessage/);
assert.doesNotMatch(timelineSource, /ThinkingItem|AssistantProcessNote/);
assert.doesNotMatch(timelineStyleSource, /\.stepRail|\.stepDot|\.processSteps/);
assert.match(traceRowSource, /useState\(defaultOpen\)/);
assert.doesNotMatch(traceRowSource, /useState\([^)]*tone === 'error'/);
const processStreamRule = timelineStyleSource.match(/\.processStream\s*\{[^}]*\}/)?.[0] ?? '';
assert.ok(processStreamRule, 'processStream CSS rule exists');
assert.doesNotMatch(processStreamRule, /overflow:\s*auto|max-height:/);
const compactSummaryRule = timelineStyleSource.match(/\.compactSummaryPanel\s*\{[^}]*\}/)?.[0] ?? '';
assert.ok(compactSummaryRule, 'compact summary CSS rule exists');
assert.doesNotMatch(compactSummaryRule, /overflow(?:-y)?:\s*(?:auto|scroll)|max-height:/);
const toolOutputRule = toolStyleSource.match(/\.output\s*\{[^}]*\}/)?.[0] ?? '';
assert.ok(toolOutputRule, 'tool output CSS rule exists');
assert.doesNotMatch(toolOutputRule, /max-height:|overflow-y:\s*(?:auto|scroll)/);
const rowErrorRule = traceRowStyleSource.match(/\.rowError\s*\{[^}]*\}/)?.[0] ?? '';
const rowWarningRule = traceRowStyleSource.match(/\.rowWarning\s*\{[^}]*\}/)?.[0] ?? '';
const processFailedRule = toolStyleSource.match(/\.processFailed\s*\{[^}]*\}/)?.[0] ?? '';
assert.ok(rowErrorRule && rowWarningRule && processFailedRule, 'compact status CSS rules exist');
assert.doesNotMatch(rowErrorRule, /background:/);
assert.doesNotMatch(rowWarningRule, /background:/);
assert.doesNotMatch(processFailedRule, /background:/);
assert.match(rowErrorRule, /var\(--ant-color-error\)/);
assert.match(processFailedRule, /var\(--ant-color-error\)/);
assert.match(timelineStyleSource, /\.processNarration\s*\{[^}]*var\(--ant-color-text-secondary\)/s);
assert.match(timelineStyleSource, /\.assistantBubble\s+:global\(\.ant-bubble-content\)\s*\{[^}]*var\(--ant-color-text\)/s);
assert.doesNotMatch(toolCardSource, /failureExcerpt/);
assert.match(workspaceSource, /data-testid=\{hasConversation \|\| isSessionSwitching \? 'conversation-scroll-container'/);

assert.doesNotMatch(toolCardSource, /toolCall\.stdout/);
assert.doesNotMatch(toolCardSource, /toolCall\.stderr/);
assert.doesNotMatch(toolCardSource, /extractWrappedOutput/);
assert.match(toolCardSource, /export function ToolItemDisclosureList/);
assert.match(toolCardSource, /data-testid="tool-item-disclosure"/);
assert.match(toolCardSource, /data-testid="tool-detail"/);
assert.match(toolCardSource, /<Drawer[\s\S]*fullContent/);
assert.match(toolCardSource, /<ToolTextPreview/);
assert.match(toolCardSource, /查看完整内容/);
assert.doesNotMatch(toolCardSource, /shouldOpenByDefault/);
assert.match(toolCardSource, /const defaultActiveKey: string\[\] = \[\];/);
assert.match(toolCardSource, /defaultActiveKey=\{\[\]\}/);
assert.match(timelineSource, /ToolItemDisclosureList|ToolTraceGroup/);
assert.match(workspaceSource, /activeSession\?\.id[\s\S]*permission\.sessionId === activeSession\.id/);

assert.match(shellSource, /id: userID,[\s\S]*status: 'success'/);
assert.match(shellSource, /const nextViewModel = await adapter\.sendPrompt\(optimisticViewModel, prompt/);
assert.doesNotMatch(shellSource, /selectConversationTimeline|timeline:/);
assert.match(adapterSource, /!isOptimisticConversationMessage\(message\)/);
assert.match(adapterSource, /runtimeActionRefreshTargets\(response\)/);
assert.doesNotMatch(adapterSource, /response\.action\.(?:source|reason|evidence|payload)/);

console.log('phase07 workbench runtime rendering smoke passed');
