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
const timelineHooksPath = resolve(repoRoot, 'client', 'src', 'features', 'timeline', 'hooks.ts');
const markdownPath = resolve(repoRoot, 'client', 'src', 'features', 'markdown', 'MarkdownMessage.tsx');
const markdownStylePath = resolve(repoRoot, 'client', 'src', 'features', 'markdown', 'MarkdownMessage.module.css');
const timelineStylePath = resolve(repoRoot, 'client', 'src', 'features', 'timeline', 'Timeline.module.css');
const toolStylePath = resolve(repoRoot, 'client', 'src', 'features', 'tools', 'ToolCallCard.module.css');
const workspacePath = resolve(repoRoot, 'client', 'src', 'features', 'workspace', 'Workspace.tsx');
const composerPath = resolve(repoRoot, 'client', 'src', 'features', 'composer', 'Composer.tsx');
const traceRowPath = resolve(repoRoot, 'client', 'src', 'features', 'timeline', 'TraceRow.tsx');
const traceRowStylePath = resolve(repoRoot, 'client', 'src', 'features', 'timeline', 'TraceRow.module.css');
const outputStreamPath = resolve(repoRoot, 'client', 'src', 'runtime', 'outputStream.ts');

const [adapterSource, typesSource, toolCardSource, shellSource, turnProjectionSource, timelineSource, disclosureSource, narrationSource, timelineHooksSource, markdownSource, markdownStyleSource, timelineStyleSource, toolStyleSource, workspaceSource, composerSource, traceRowSource, traceRowStyleSource, outputStreamSource] = await Promise.all([
  readFile(adapterPath, 'utf8'),
  readFile(typesPath, 'utf8'),
  readFile(toolCardPath, 'utf8'),
  readFile(shellPath, 'utf8'),
  readFile(turnProjectionPath, 'utf8'),
  readFile(timelinePath, 'utf8'),
  readFile(disclosurePath, 'utf8'),
  readFile(narrationPath, 'utf8'),
  readFile(timelineHooksPath, 'utf8'),
  readFile(markdownPath, 'utf8'),
  readFile(markdownStylePath, 'utf8'),
  readFile(timelineStylePath, 'utf8'),
  readFile(toolStylePath, 'utf8'),
  readFile(workspacePath, 'utf8'),
  readFile(composerPath, 'utf8'),
  readFile(traceRowPath, 'utf8'),
  readFile(traceRowStylePath, 'utf8'),
  readFile(outputStreamPath, 'utf8'),
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
assert.doesNotMatch(disclosureSource, /shouldAutoOpenProcess|<Collapse|activeKey=/);
assert.match(disclosureSource, /\? '处理中' : '处理完成'/);
assert.doesNotMatch(disclosureSource, /formatElapsed|formatDuration|failedCount|subagentCount/);
assert.match(disclosureSource, /isRedundantActivePlaceholder/);
assert.doesNotMatch(disclosureSource, /正在探索/);
assert.match(disclosureSource, /data-testid="process-stream"/);
assert.match(disclosureSource, /className=\{styles\.processTraceHeader\}/);
assert.match(narrationSource, /data-testid="process-narration"/);
assert.match(narrationSource, /<MarkdownMessage/);
assert.match(narrationSource, /variant="process"/);
assert.match(markdownSource, /data-markdown-variant=\{resolvedVariant\}/);
assert.match(markdownStyleSource, /\.finalMarkdown\s*\{[^}]*var\(--ant-color-text\)[^}]*font-size:\s*15px/s);
assert.match(markdownStyleSource, /\.processMarkdown\s*\{[^}]*var\(--ant-color-text-secondary\)[^}]*font-size:\s*14px/s);
assert.match(timelineHooksSource, /autoOpen \? 'active' : 'settled'/);
assert.match(timelineHooksSource, /state\.resetKey === lifecycleKey \? state\.manual \?\? autoOpen : autoOpen/);
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
assert.doesNotMatch(workspaceSource, /startMark|startCapabilities|梳理需求|执行任务|检查结果/);
assert.match(workspaceSource, /className=\{styles\.startIntro\}/);
assert.match(workspaceSource, /const composerDraftTarget = isDraftSurface/);
assert.match(composerSource, /<Flex align="center" className=\{styles\.limitBar\}/);
assert.match(timelineStyleSource, /\.processTraceStandalone/);

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
assert.ok(outputStreamSource.indexOf('Events.On(SESSION_OUTPUT_STREAM_EVENT') < outputStreamSource.indexOf('StartSessionOutputStream({'), 'session listener is registered before stream start');
assert.ok(adapterSource.indexOf("Events.On(eventName") < adapterSource.indexOf('StartRuntimeEventStream({ streamId'), 'runtime listener is registered before stream start');
assert.doesNotMatch(outputStreamSource, /EventSource|SessionOutputEvents/);
assert.match(workspaceSource, /sessionTodos\?\.turnId\s*\?\s*conversationTurns\.find/);
assert.doesNotMatch(workspaceSource, /todoTurn[\s\S]{0,240}\?\?\s*conversationTurns/);
assert.match(adapterSource, /const target = current\.conversationTarget/);
assert.match(adapterSource, /target\.kind === 'session' \? target\.sessionId : undefined/);
assert.match(adapterSource, /conversationTarget: \{ kind: 'session', sessionId: responseSessionID \}/);
assert.match(adapterSource, /bindDraftToCurrentProject\(await hydrateWorkbench\(nextBase, bridge\)\)/);
assert.doesNotMatch(adapterSource, /forceDraftChatSubmit|bridge\.NewChat\(''\)/);
assert.match(shellSource, /createConversationSubmitQueue\(\)/);
assert.match(shellSource, /promptSubmitQueueRef\.current\.enqueue/);
assert.match(shellSource, /const conversationEpoch = \+\+sessionMutationSeqRef\.current/);
assert.match(shellSource, /sessionMutationSeqRef\.current \+= 1;[\s\S]*viewModelRef\.current = nextViewModel/);
assert.match(adapterSource, /retargetOutputStore\(current\.outputStore, responseSessionID\)/);
assert.match(adapterSource, /bridge\.SessionOutput!\(responseSessionID, \{ snapshot: true \}\)/);
assert.match(adapterSource, /hydrateOutputStore\(initialOutputSnapshot, adoptedOutputStore\)/);
assert.doesNotMatch(adapterSource, /role: 'assistant' as const,[\s\S]{0,160}status: 'loading' as const/);

assert.match(shellSource, /id: userID,[\s\S]*status: 'success'/);
assert.match(workspaceSource, /selectSessionTodos\(viewModel\.outputStore, activeSession\?\.id\)/);
assert.doesNotMatch(shellSource, /todos: mapRuntimeTodoSummary/);
assert.doesNotMatch(adapterSource, /SessionTodos|TurnTodos|hydrateTodos/);
assert.doesNotMatch(shellSource, /正在生成回复|loadingID/);
assert.match(shellSource, /const nextViewModel = await adapter\.sendPrompt\(optimisticViewModel, prompt/);
assert.doesNotMatch(shellSource, /selectConversationTimeline|timeline:/);
assert.match(adapterSource, /!isOptimisticConversationMessage\(message\)/);
assert.match(adapterSource, /runtimeActionRefreshTargets\(response\)/);
assert.doesNotMatch(adapterSource, /response\.action\.(?:source|reason|evidence|payload)/);

console.log('phase07 workbench runtime rendering smoke passed');
