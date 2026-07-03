import { useEffect, useState } from 'react';
import {
  BranchesOutlined,
  CheckOutlined,
  CopyOutlined,
  DownOutlined,
  MessageOutlined,
  SafetyCertificateOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { Button, Collapse, Progress, Tag, Tooltip, message } from 'antd';
import Bubble from '@ant-design/x/es/bubble';
import type { ConversationTimelineItemViewModel, HookExecutionSummaryViewModel, HookExecutionViewModel, ToolCallViewModel } from '../../runtime/workbenchTypes.ts';
import { ThinkingItem } from './ThinkingItem.tsx';
import { ToolCallCard, QuietToolRowList } from '../tools/ToolCallCard.tsx';
import { MarkdownMessage } from '../markdown/MarkdownMessage.tsx';
import { HookExecutionDetailDrawer } from '../hooks/HookExecutionDetailDrawer.tsx';
import { HookTimelineRow } from '../hooks/HookTimelineRow.tsx';
import { InlineExpandable, TraceRow } from './TraceRow.tsx';
import { CompactDivider } from './CompactDivider.tsx';
import { useLatchedOpen, useMinDisplay, useRatchetCounts } from './hooks.ts';
import styles from './Timeline.module.css';

interface TimelineProps {
  items: ConversationTimelineItemViewModel[];
  hookExecutions?: HookExecutionSummaryViewModel;
  onAgentTaskOpen?: (taskID: string) => void;
  onHookExecutionLoad?: (executionID: string) => Promise<HookExecutionViewModel>;
}

type RenderTimelineItem = ConversationTimelineItemViewModel | ToolCallGroupRenderItem | ToolCallSummaryRenderItem;

interface ToolCallGroupRenderItem {
  id: string;
  kind: 'tool_call_group';
  turnId?: string;
  toolCalls: ToolCallViewModel[];
}

interface ToolCallSummaryRenderItem {
  id: string;
  kind: 'tool_call_summary';
  turnId?: string;
  toolCalls: ToolCallViewModel[];
}

interface TimelineTurnBlock {
  id: string;
  turnId?: string;
  userMessage?: ConversationTimelineItemViewModel;
  explorationSummary?: ConversationTimelineItemViewModel;
  processItems: ConversationTimelineItemViewModel[];
  finalMessage?: ConversationTimelineItemViewModel;
  looseItems: ConversationTimelineItemViewModel[];
  status?: string;
  startedAt?: number;
  finishedAt?: number;
}

export function Timeline({ items, hookExecutions, onAgentTaskOpen, onHookExecutionLoad }: TimelineProps) {
  const [messageApi, messageContextHolder] = message.useMessage();
  const [selectedHookExecution, setSelectedHookExecution] = useState<HookExecutionViewModel | undefined>();
  const blocks = buildTurnBlocks(items);

  return (
    <div className={styles.timeline} data-testid="conversation-timeline">
      {messageContextHolder}
      {blocks.map((block) => (
        <TurnBlock
          key={block.id}
          block={block}
          hookExecutions={hookExecutions}
          messageApi={messageApi}
          onAgentTaskOpen={onAgentTaskOpen}
          onHookOpen={setSelectedHookExecution}
        />
      ))}
      <HookExecutionDetailDrawer
        executionId={selectedHookExecution?.id}
        fallback={selectedHookExecution}
        open={Boolean(selectedHookExecution)}
        onClose={() => setSelectedHookExecution(undefined)}
        onLoad={onHookExecutionLoad}
      />
    </div>
  );
}

function TurnBlock({
  block,
  hookExecutions,
  messageApi,
  onAgentTaskOpen,
  onHookOpen,
}: {
  block: TimelineTurnBlock;
  hookExecutions?: HookExecutionSummaryViewModel;
  messageApi: ReturnType<typeof message.useMessage>[0];
  onAgentTaskOpen?: (taskID: string) => void;
  onHookOpen?: (execution: HookExecutionViewModel) => void;
}) {
  const promptHooks = block.processItems.some((item) => item.kind === 'hook_run') ? [] : highSignalHooks(hookExecutions, block.turnId).filter((execution) => !execution.toolCallId);
  return (
    <section className={styles.turnBlock} data-testid="timeline-turn-block" data-turn-id={block.turnId}>
      {block.userMessage && <TimelineMessage item={block.userMessage} messageApi={messageApi} />}
      {promptHooks.map((execution) => (
        <HookTimelineRow key={execution.id} execution={execution} onOpen={onHookOpen} />
      ))}
      {block.processItems.length > 0 && <ProcessTrace block={block} hookExecutions={hookExecutions} onAgentTaskOpen={onAgentTaskOpen} onHookOpen={onHookOpen} />}
      {block.finalMessage && <TimelineMessage item={block.finalMessage} messageApi={messageApi} />}
      {block.looseItems.map((item) => (
        <TimelineProcessItem key={item.id} hookExecutions={hookExecutions} item={item} onAgentTaskOpen={onAgentTaskOpen} onHookOpen={onHookOpen} />
      ))}
    </section>
  );
}

function ProcessTrace({
  block,
  hookExecutions,
  onAgentTaskOpen,
  onHookOpen,
}: {
  block: TimelineTurnBlock;
  hookExecutions?: HookExecutionSummaryViewModel;
  onAgentTaskOpen?: (taskID: string) => void;
  onHookOpen?: (execution: HookExecutionViewModel) => void;
}) {
  const traceLabel = processTraceSummary(block);
  const groupedItems = compactProcessItems(block.processItems);
  const autoOpen = shouldOpenProcessTrace(block);
  const [open, setOpen] = useLatchedOpen(autoOpen, block.turnId);
  return (
    <section className={styles.processTrace} data-testid="process-trace" data-process-label={traceLabel} data-process-status={block.status}>
      <Collapse
        ghost
        size="small"
        activeKey={open ? ['trace'] : []}
        expandIcon={({ isActive }) => <DownOutlined rotate={isActive ? 180 : 0} />}
        items={[
          {
            key: 'trace',
            label: <ProcessTraceLabel block={block} />,
            children: (
              <div className={styles.processSteps}>
                {groupedItems.map((item) => (
                  <div key={item.id} className={styles.stepRail} data-step-status={stepStatus(item)}>
                    <span className={styles.stepDot} aria-hidden="true" />
                    <div className={styles.stepContent}>
                      <TimelineProcessItem hookExecutions={hookExecutions} item={item} onAgentTaskOpen={onAgentTaskOpen} onHookOpen={onHookOpen} />
                    </div>
                  </div>
                ))}
              </div>
            ),
          },
        ]}
        onChange={(keys) => setOpen(Array.isArray(keys) ? keys.includes('trace') : keys === 'trace')}
      />
    </section>
  );
}

function stepStatus(item: RenderTimelineItem): 'running' | 'failed' | 'done' {
  if (item.kind === 'tool_call_group' || item.kind === 'tool_call_summary') {
    if (item.toolCalls.some((call) => isActiveTurnStatus(call.status))) {
      return 'running';
    }
    if (item.toolCalls.some((call) => isFailedStatus(call.status))) {
      return 'failed';
    }
    return 'done';
  }
  const status = item.status;
  if (isActiveTurnStatus(status)) {
    return 'running';
  }
  if (isFailedStatus(status)) {
    return 'failed';
  }
  return 'done';
}

function isFailedStatus(status?: string) {
  return status === 'failed' || status === 'denied' || status === 'cancelled' || status === 'interrupted';
}

function ProcessTraceLabel({ block }: { block: TimelineTurnBlock }) {
  const exploration = block.explorationSummary;
  const summary = exploration?.exploration;
  const rawCounts = summary?.toolCounts ?? exploration?.displayCounts;
  const status = summary?.status ?? block.status;
  const counts = useRatchetCounts(rawCounts, block.turnId);
  const verb = useMinDisplay(explorationStatusVerb(status, summary?.failedCount), 700);
  if (counts && counts.length > 0) {
    return (
      <span className={styles.processTraceLabel} data-testid="process-trace-label" data-exploration-status={status}>
        <span>{verb}</span>
        {counts.map((count) => (
          <span key={count.kind}>{explorationCountLabel(count)}</span>
        ))}
        {summary?.subagentCount ? <span>· {summary.subagentCount} 个子任务</span> : null}
        {summary?.elapsedMs ? <span>{formatElapsed(summary.elapsedMs)}</span> : null}
      </span>
    );
  }
  const { duration, label, toolCount } = processTraceSummaryParts(block);
  return (
    <span className={styles.processTraceLabel}>
      <span>{label}</span>
      {duration && <span>{duration}</span>}
      {toolCount > 0 && <span>{toolCount} 个工具</span>}
    </span>
  );
}

function explorationStatusVerb(status?: string, failedCount?: number) {
  if (failedCount && failedCount > 0) {
    return '部分失败';
  }
  switch (status) {
    case 'exploring':
      return '正在探索';
    case 'done':
      return '已完成';
    case 'failed':
      return '失败';
    case 'interrupted':
      return '已中断';
    default:
      return '探索';
  }
}

function explorationCountLabel(count: { kind: string; count: number; failed?: number }) {
  const base = (() => {
    switch (count.kind) {
      case 'file_read':
        return count.count === 1 ? '读取 1 个文件' : `读取 ${count.count} 个文件`;
      case 'file_search':
        return count.count === 1 ? '搜索 1 次' : `搜索 ${count.count} 次`;
      case 'shell':
        return count.count === 1 ? '运行 1 条命令' : `运行 ${count.count} 条命令`;
      case 'file_edit':
        return count.count === 1 ? '编辑 1 个文件' : `编辑 ${count.count} 个文件`;
      case 'file_write':
        return count.count === 1 ? '写入 1 个文件' : `写入 ${count.count} 个文件`;
      case 'agent_task':
        return count.count === 1 ? '1 个子任务' : `${count.count} 个子任务`;
      default:
        return count.count === 1 ? '1 个工具' : `${count.count} 个工具`;
    }
  })();
  if (count.failed && count.failed > 0 && count.failed < count.count) {
    return `${base}(${count.failed} 失败)`;
  }
  if (count.failed && count.failed === count.count) {
    return `${base}(失败)`;
  }
  return base;
}

function formatElapsed(ms: number) {
  if (ms < 1000) {
    return `${ms}ms`;
  }
  if (ms < 60_000) {
    return `${(ms / 1000).toFixed(1)}s`;
  }
  const minutes = Math.floor(ms / 60_000);
  const seconds = Math.round((ms % 60_000) / 1000);
  return `${minutes}m${seconds}s`;
}

function TimelineProcessItem({
  item,
  hookExecutions,
  onAgentTaskOpen,
  onHookOpen,
}: {
  item: RenderTimelineItem;
  hookExecutions?: HookExecutionSummaryViewModel;
  onAgentTaskOpen?: (taskID: string) => void;
  onHookOpen?: (execution: HookExecutionViewModel) => void;
}) {
  if (item.kind === 'tool_call_summary') {
    return <ToolRunSummary item={item} onAgentTaskOpen={onAgentTaskOpen} />;
  }
  if (item.kind === 'tool_call_group') {
    return (
      <>
        <ToolCallCard toolCalls={item.toolCalls} onAgentTaskOpen={onAgentTaskOpen} />
        {item.toolCalls.flatMap((toolCall) => highSignalHooks(hookExecutions, item.turnId, toolCall.id)).map((execution) => (
          <HookTimelineRow key={execution.id} execution={execution} onOpen={onHookOpen} />
        ))}
      </>
    );
  }
  if ((item.kind === 'tool_call' || item.kind === 'tool_group') && item.toolCall) {
    return (
      <>
        <ToolCallCard toolCall={item.toolCall} onAgentTaskOpen={onAgentTaskOpen} />
        {highSignalHooks(hookExecutions, item.turnId, item.toolCall.id).map((execution) => (
          <HookTimelineRow key={execution.id} execution={execution} onOpen={onHookOpen} />
        ))}
      </>
    );
  }
  if ((item.kind === 'permission' || item.kind === 'permission_request') && item.permission) {
    return <PermissionTraceRow item={item} />;
  }
  if (item.kind === 'thinking' || item.kind === 'assistant_thinking') {
    return <ThinkingItem item={item} />;
  }
  if (item.kind === 'progress' || item.kind === 'turn_progress') {
    const detail = progressDetail(item);
    return (
      <div className={styles.progress} data-testid="turn-progress" data-progress-status={item.status}>
        <div>{progressLabel(item.status)}</div>
        {detail && <div className={styles.progressDetail}>{detail}</div>}
      </div>
    );
  }
  if (item.kind === 'turn_terminal') {
    return <WorkflowNoticeRow item={item} />;
  }
  if (item.kind === 'diagnostic' || item.kind === 'diagnostic_warning') {
    return <TurnDiagnosticWarning item={item} />;
  }
  if (item.kind === 'hook_run' || item.kind === 'todo_summary' || item.kind === 'recovery_notice') {
    return <WorkflowNoticeRow item={item} />;
  }
  if (item.kind === 'compact_boundary') {
    return <CompactDivider item={item} />;
  }
  if (isContextGovernanceItem(item)) {
    return <ContextGovernanceRow item={item} />;
  }
  if (item.kind === 'agent_task') {
    return item.agentTask ? <AgentTaskTimelineRow item={item} onAgentTaskOpen={onAgentTaskOpen} /> : <WorkflowNoticeRow item={item} />;
  }
  if (item.kind === 'message' || item.kind === 'assistant_message') {
    return <AssistantProcessNote item={item} />;
  }
  return null;
}

function WorkflowNoticeRow({ item }: { item: ConversationTimelineItemViewModel }) {
  const failed = item.status === 'failed' || item.status === 'interrupted' || Boolean(item.error);
  return (
    <TraceRow
      dataAttrs={{ 'data-workflow-kind': item.kind, 'data-workflow-status': item.status }}
      extra={item.summary || item.error || item.content ? <span>{item.error || item.summary || item.content}</span> : null}
      icon={failed ? <WarningOutlined /> : <BranchesOutlined />}
      meta={item.status ? <Tag color={failed ? 'error' : 'default'}>{item.status}</Tag> : null}
      testId="timeline-workflow-row"
      title={item.title || workflowNoticeTitle(item.kind)}
      tone={failed ? 'error' : 'default'}
    />
  );
}

function workflowNoticeTitle(kind: string) {
  switch (kind) {
    case 'hook_run':
      return 'Hook';
    case 'todo_summary':
      return 'Todo';
    case 'recovery_notice':
      return 'Recovery';
    case 'turn_terminal':
      return 'Turn ended';
    default:
      return 'Workflow';
  }
}

function ContextGovernanceRow({ item }: { item: ConversationTimelineItemViewModel }) {
  const failed = item.status === 'failed' || Boolean(item.error);
  return (
    <TraceRow
      dataAttrs={{ 'data-context-kind': item.kind, 'data-context-status': item.status }}
      extra={item.summary || item.error ? <span>{item.error || item.summary}</span> : null}
      icon={failed ? <WarningOutlined /> : <BranchesOutlined />}
      meta={item.status ? <Tag color={failed ? 'error' : 'default'}>{item.status}</Tag> : null}
      testId="timeline-context-governance-row"
      title={contextGovernanceTitle(item)}
      tone={failed ? 'error' : 'default'}
    />
  );
}

function ToolRunSummary({ item, onAgentTaskOpen }: { item: ToolCallSummaryRenderItem; onAgentTaskOpen?: (taskID: string) => void }) {
  const duration = toolCallsDuration(item.toolCalls);
  const kinds = summarizeToolKinds(item.toolCalls);
  return (
    <TraceRow
      expandable
      icon={<CheckOutlined />}
      meta={
        <>
          {duration && <span>{duration}</span>}
          {kinds && <span>{kinds}</span>}
        </>
      }
      testId="tool-run-summary"
      title={`已完成 ${item.toolCalls.length} 个工具`}
    >
      <QuietToolRowList toolCalls={item.toolCalls} onAgentTaskOpen={onAgentTaskOpen} />
    </TraceRow>
  );
}

function TimelineMessage({ item, messageApi }: { item: ConversationTimelineItemViewModel; messageApi: ReturnType<typeof message.useMessage>[0] }) {
  const streaming = Boolean(item.streaming);
  const displayContent = streaming ? completePartialMarkdown(item.content ?? '') : item.content;
  return (
    <Bubble
      className={[
        item.role === 'user' ? styles.userBubble : styles.assistantBubble,
        streaming ? styles.streamingBubble : undefined,
      ].filter(Boolean).join(' ')}
      typing={false}
      content={
        <span data-testid="timeline-message" data-streaming={streaming ? 'true' : undefined}>
          <MarkdownMessage content={displayContent} role={item.role} />
          {streaming ? <span className={styles.streamingCursor} aria-hidden="true">▍</span> : null}
        </span>
      }
      placement={item.role === 'user' ? 'end' : 'start'}
      variant={item.role === 'user' ? 'filled' : 'borderless'}
      footer={
        (item.role === 'user' || item.role === 'assistant') && isCompleteMessage(item) ? (
          <MessageFooter align={item.role === 'user' ? 'end' : 'start'} content={item.content ?? ''} createdAt={item.createdAt} messageApi={messageApi} />
        ) : item.status === 'error' ? (
          <Tag color="error">失败</Tag>
        ) : undefined
      }
    />
  );
}

// completePartialMarkdown closes obviously unbalanced fenced code blocks so
// the ReactMarkdown parser doesn't get stuck mid-stream. It only handles the
// common case of an odd number of ``` fences; everything else falls through
// unchanged.
function completePartialMarkdown(content: string): string {
  const fenceMatches = content.match(/```/g);
  if (fenceMatches && fenceMatches.length % 2 === 1) {
    return content + '\n```';
  }
  return content;
}

function AssistantProcessNote({ item }: { item: ConversationTimelineItemViewModel }) {
  const content = item.content?.trim();
  if (!content) {
    return null;
  }
  return (
    <TraceRow expandable icon={<MessageOutlined />} testId="timeline-process-note" title={summarizeProcessNote(content)}>
      <MarkdownMessage content={content} role="assistant" />
    </TraceRow>
  );
}

function PermissionTraceRow({ item }: { item: ConversationTimelineItemViewModel }) {
  const permission = item.permission;
  if (!permission) {
    return null;
  }
  const failed = permission.status === 'denied' || permission.status === 'cancelled' || permission.status === 'expired';
  const reason = permission.reason || permission.policyReason;
  return (
    <TraceRow
      dataAttrs={{ 'data-permission-status': permission.status }}
      extra={reason ? <span>{reason}</span> : null}
      icon={failed ? <WarningOutlined /> : <SafetyCertificateOutlined />}
      testId="permission-trace-row"
      title={
        <>
          {permissionStatusLabel(permission.status)}
          <code className={styles.inlineCode}>{permission.target || permission.path || permission.toolName}</code>
        </>
      }
      tone={failed ? 'error' : 'default'}
    />
  );
}

function AgentTaskTimelineRow({ item, onAgentTaskOpen }: { item: ConversationTimelineItemViewModel; onAgentTaskOpen?: (taskID: string) => void }) {
  const task = item.agentTask;
  if (!task) {
    return null;
  }
  const refs = [...(task.outputRefs ?? []), ...(task.artifactRefs ?? [])];
  const summary = task.resultSummary || task.promptSummary || '';
  const failed = task.status === 'failed' || task.status === 'interrupted';
  const metaLine = [task.role || task.kind, task.provider && task.model ? `${task.provider}/${task.model}` : task.model, task.childSessionId ? `child ${task.childSessionId}` : undefined]
    .filter(Boolean)
    .join(' / ');
  return (
    <TraceRow
      clickable
      dataAttrs={{ 'data-task-id': task.id }}
      extra={
        <div className={styles.agentTaskExtra}>
          <Progress percent={task.progress ?? 0} size="small" showInfo={false} />
          {metaLine ? <div className={styles.agentTaskMetaLine}>{metaLine}</div> : null}
          {summary ? <InlineExpandable summary={summarizeProcessNote(summary)}>{summary}</InlineExpandable> : null}
          {refs.length ? <div className={styles.agentTaskRefsLine}>{refs.slice(0, 3).join(' / ')}</div> : null}
        </div>
      }
      icon={<BranchesOutlined />}
      meta={<Tag color={agentTaskStatusColor(task.status)}>{task.status}</Tag>}
      testId="timeline-agent-task-row"
      title={task.title || task.id}
      tone={failed ? 'error' : 'default'}
      onRowClick={() => onAgentTaskOpen?.(task.id)}
    />
  );
}

function TurnDiagnosticWarning({ item }: { item: ConversationTimelineItemViewModel }) {
  const missingArtifacts = item.diagnostics?.missingArtifacts ?? [];
  if (!item.diagnostics?.warning && !item.summary && missingArtifacts.length === 0) {
    return null;
  }
  return (
    <TraceRow
      extra={<span>{missingArtifacts.length > 0 ? formatMissingArtifacts(missingArtifacts) : item.summary}</span>}
      icon={<WarningOutlined />}
      testId="turn-diagnostic-warning"
      title={diagnosticWarningTitle(item)}
      tone="warning"
    />
  );
}

function buildTurnBlocks(items: ConversationTimelineItemViewModel[]): TimelineTurnBlock[] {
  const blocks: TimelineTurnBlock[] = [];
  const blockByTurn = new Map<string, TimelineTurnBlock>();

  const ensureBlock = (turnId?: string, fallbackId?: string) => {
    const id = turnId || fallbackId || `loose:${blocks.length}`;
    if (turnId && blockByTurn.has(turnId)) {
      return blockByTurn.get(turnId)!;
    }
    const block: TimelineTurnBlock = {
      id: turnId ? `turn:${turnId}` : id,
      turnId,
      processItems: [],
      looseItems: [],
    };
    blocks.push(block);
    if (turnId) {
      blockByTurn.set(turnId, block);
    }
    return block;
  };

  for (const item of items) {
    const block = ensureBlock(item.turnId, item.id);
    block.startedAt = minDefined(block.startedAt, item.createdAt);
    block.finishedAt = maxDefined(block.finishedAt, item.updatedAt || item.createdAt);
    block.status = mergeTurnStatus(block.status, item.status);

    if ((item.kind === 'message' || item.kind === 'user_message') && item.role === 'user') {
      block.userMessage = block.userMessage ?? item;
      continue;
    }
    if (item.kind === 'exploration_summary') {
      // Runtime-owned per-turn exploration counters go straight to the
      // process trace header; they should never show up as an inline
      // trace row.
      block.explorationSummary = item;
      continue;
    }
    if ((item.kind === 'message' || item.kind === 'assistant_message') && item.role === 'assistant') {
      if (isFinalAssistantMessage(item)) {
        block.finalMessage = item;
      } else {
        block.processItems.push(item);
      }
      continue;
    }
    if (isProcessItem(item)) {
      block.processItems.push(item);
      continue;
    }
    block.looseItems.push(item);
  }

  for (const block of blocks) {
    if (block.finalMessage && !isActiveTurnStatus(block.status)) {
      block.status = block.finalMessage.status === 'error' ? 'failed' : 'completed';
    }
  }

  return blocks.filter((block) => block.userMessage || block.finalMessage || block.processItems.length > 0 || block.looseItems.length > 0);
}

function isProcessItem(item: ConversationTimelineItemViewModel) {
  return (
    item.kind === 'thinking' ||
    item.kind === 'assistant_thinking' ||
    item.kind === 'tool_call' ||
    item.kind === 'tool_group' ||
    item.kind === 'permission' ||
    item.kind === 'permission_request' ||
    item.kind === 'progress' ||
    item.kind === 'turn_progress' ||
    item.kind === 'diagnostic' ||
    item.kind === 'diagnostic_warning' ||
    item.kind === 'hook_run' ||
    item.kind === 'todo_summary' ||
    item.kind === 'recovery_notice' ||
    item.kind === 'agent_task' ||
    item.kind === 'turn_terminal' ||
    isContextGovernanceItem(item)
  );
}

function highSignalHooks(summary?: HookExecutionSummaryViewModel, turnId?: string, toolCallId?: string) {
  if (!turnId && !toolCallId) {
    return [];
  }
  return (summary?.items ?? []).filter((execution) => {
    if (turnId && execution.turnId && execution.turnId !== turnId) {
      return false;
    }
    if (toolCallId) {
      return execution.toolCallId === toolCallId && isHighSignalHook(execution);
    }
    return !execution.toolCallId && isHighSignalHook(execution);
  });
}

function isHighSignalHook(execution: HookExecutionViewModel) {
  return (
    execution.status === 'blocked' ||
    execution.status === 'denied' ||
    execution.status === 'failed' ||
    execution.inputRewritten ||
    execution.contextInjected
  );
}

function isFinalAssistantMessage(item: ConversationTimelineItemViewModel) {
  return item.role === 'assistant' && (item.kind === 'message' || item.kind === 'assistant_message') && item.phase !== 'intermediate' && (item.content?.trim() || item.status === 'error' || item.status === 'completed');
}

function shouldOpenProcessTrace(block: TimelineTurnBlock) {
  const explorationStatus = block.explorationSummary?.exploration?.status;
  const failedCount = block.explorationSummary?.exploration?.failedCount ?? 0;
  if (explorationStatus === 'exploring' || failedCount > 0) {
    return true;
  }
  if (!block.finalMessage) {
    return true;
  }
  return block.processItems.some((item) => isActiveTurnStatus(item.status));
}

function isActiveTurnStatus(status?: string) {
  return status === 'running' || status === 'queued' || status === 'waiting_permission';
}

function isContextGovernanceItem(item: ConversationTimelineItemViewModel) {
	return (
		item.kind === 'compact_boundary' ||
		item.kind === 'context_source'
	);
}

function contextGovernanceTitle(item: ConversationTimelineItemViewModel) {
	switch (item.kind) {
		case 'compact_boundary':
			return `Context compact${item.title ? `: ${item.title}` : ''}`;
		case 'context_source':
			return `Context source${item.title ? `: ${item.title}` : ''}`;
    default:
      return item.title || 'Context governance';
  }
}

function processTraceLabel(status?: string) {
  switch (status) {
    case 'running':
    case 'queued':
    case 'waiting_permission':
      return '处理中';
    case 'failed':
      return '处理失败';
    case 'denied':
      return '已拒绝';
    case 'cancelled':
      return '已取消';
    default:
      return '已处理';
  }
}

function processTraceSummary(block: TimelineTurnBlock) {
  const { duration, label, toolCount } = processTraceSummaryParts(block);
  return [label, duration, toolCount > 0 ? `${toolCount} 个工具` : undefined, block.turnId].filter(Boolean).join(' ');
}

function processTraceSummaryParts(block: TimelineTurnBlock) {
  return {
    duration: blockDuration(block),
    label: processTraceLabel(block.status),
    toolCount: block.processItems.filter((item) => item.kind === 'tool_call' || item.kind === 'tool_group').length,
  };
}

function blockDuration(block: TimelineTurnBlock) {
  if (!block.startedAt || !block.finishedAt) {
    return '';
  }
  return formatDuration(block.startedAt, block.finishedAt);
}

function mergeTurnStatus(current?: string, next?: string) {
  if (!next) {
    return current;
  }
  const rank: Record<string, number> = {
    denied: 5,
    failed: 5,
    waiting_permission: 4,
    running: 3,
    queued: 2,
    cancelled: 1,
    success: 0,
    completed: 0,
  };
  if (!current) {
    return next;
  }
  return (rank[next] ?? 0) > (rank[current] ?? 0) ? next : current;
}

function compactProcessItems(items: ConversationTimelineItemViewModel[]): RenderTimelineItem[] {
  const compacted: RenderTimelineItem[] = [];
  let quietSummary: ToolCallSummaryRenderItem | undefined;
  let quietSummaryRoundKey: string | undefined;

  for (const item of items) {
    if (item.kind === 'tool_call' && item.toolCall && isQuietCompletedToolCall(item.toolCall)) {
      const roundKey = toolCallRoundKey(item);
      if (!quietSummary || quietSummaryRoundKey !== roundKey) {
        quietSummary = {
          id: `tool-summary:${roundKey}`,
          kind: 'tool_call_summary',
          turnId: item.turnId,
          toolCalls: [],
        };
        quietSummaryRoundKey = roundKey;
        compacted.push(quietSummary);
      }
      quietSummary.toolCalls.push(item.toolCall);
      continue;
    }
    quietSummary = undefined;
    quietSummaryRoundKey = undefined;
    compacted.push(item);
  }

  return groupAdjacentToolCalls(compacted);
}

function groupAdjacentToolCalls(items: RenderTimelineItem[]): RenderTimelineItem[] {
  const grouped: RenderTimelineItem[] = [];
  let pending: ConversationTimelineItemViewModel[] = [];

  const flush = () => {
    if (pending.length === 0) {
      return;
    }
    if (pending.length === 1) {
      grouped.push(pending[0]);
      pending = [];
      return;
    }
    grouped.push({
      id: `tool-group:${pending.map((item) => item.toolCallId || item.id).join(':')}`,
      kind: 'tool_call_group',
      turnId: pending[0].turnId,
      toolCalls: pending.map((item) => item.toolCall).filter((toolCall): toolCall is ToolCallViewModel => Boolean(toolCall)),
    });
    pending = [];
  };

  for (const item of items) {
    if (item.kind === 'tool_call' && item.toolCall) {
      const previous = pending[pending.length - 1];
      if (!previous || (previous.turnId === item.turnId && timelineToolKind(previous.toolCall) === timelineToolKind(item.toolCall) && previous.toolCall?.status === item.toolCall.status)) {
        pending.push(item);
        continue;
      }
    }
    flush();
    grouped.push(item);
  }
  flush();

  return grouped;
}

function isQuietCompletedToolCall(toolCall: ToolCallViewModel) {
  const status = toolCall.status;
  return toolCall.quiet === true && (status === 'completed' || status === 'success') && !toolCall.error;
}

function toolCallRoundKey(item: ConversationTimelineItemViewModel) {
  return item.messageId || item.toolCall?.id || item.toolCallId || item.id;
}

function toolCallsDuration(toolCalls: ToolCallViewModel[]) {
  const startedAt = toolCalls.reduce<number | undefined>((current, toolCall) => minDefined(current, toolCall.startedAt), undefined);
  const finishedAt = toolCalls.reduce<number | undefined>((current, toolCall) => maxDefined(current, toolCall.finishedAt), undefined);
  return startedAt && finishedAt ? formatDuration(startedAt, finishedAt) : '';
}

function summarizeToolKinds(toolCalls: ToolCallViewModel[]) {
  const counts = new Map<string, number>();
  for (const toolCall of toolCalls) {
    const kind = timelineToolKind(toolCall);
    counts.set(kind, (counts.get(kind) ?? 0) + 1);
  }
  return [...counts.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, 3)
    .map(([kind, count]) => `${toolKindLabel(kind)} ${count}`)
    .join(' / ');
}

function toolKindLabel(kind: string) {
  switch (kind) {
    case 'file_read':
      return '读取';
    case 'file_write':
      return '写入';
    case 'file_edit':
      return '编辑';
    case 'file_search':
      return '搜索';
    case 'shell':
      return '命令';
    default:
      return '工具';
  }
}

function timelineToolKind(toolCall?: ToolCallViewModel) {
  if (!toolCall) {
    return 'generic';
  }
  return toolCall.display?.kind || toolCall.kind || 'generic';
}

function agentTaskStatusColor(status?: string) {
  switch (status) {
    case 'queued':
    case 'running':
      return 'processing';
    case 'completed':
      return 'success';
    case 'failed':
    case 'interrupted':
      return 'error';
    default:
      return 'default';
  }
}

function diagnosticWarningTitle(item: ConversationTimelineItemViewModel) {
  switch (item.diagnostics?.warningReason) {
    case 'produced_artifact_missing_on_disk':
      return '工具已报告生成文件，但磁盘上不存在';
    case 'expected_artifact_not_produced':
      return '期望文件未由工具生成';
    default:
      return item.diagnostics?.warning ?? '产物警告';
  }
}

function isCompleteMessage(item: ConversationTimelineItemViewModel) {
  if (item.streaming) {
    return false;
  }
  // Runtime-echoed items report 'completed'; optimistic/legacy rows use
  // 'success'. Both are settled messages whose footer (copy + time) may show.
  return item.status === 'success' || item.status === 'error' || item.status === 'completed';
}

function MessageFooter({
  align,
  content,
  createdAt,
  messageApi,
}: {
  align: 'start' | 'end';
  content: string;
  createdAt?: number;
  messageApi: ReturnType<typeof message.useMessage>[0];
}) {
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!copied) {
      return undefined;
    }
    const timer = window.setTimeout(() => setCopied(false), 1200);
    return () => window.clearTimeout(timer);
  }, [copied]);

  const copyMessage = async () => {
    try {
      await copyText(content);
      setCopied(true);
      void messageApi.success('已复制');
    } catch {
      void messageApi.error('复制失败');
    }
  };

  return (
    <div className={`${styles.messageFooter} ${align === 'end' ? styles.userMessageFooter : styles.assistantMessageFooter}`}>
      <span className={styles.messageTime}>{formatMessageTime(createdAt)}</span>
      <Tooltip title={copied ? '已复制' : '复制'}>
        <Button
          aria-label={copied ? '已复制' : '复制消息'}
          className={styles.copyButton}
          icon={copied ? <CheckOutlined /> : <CopyOutlined />}
          size="small"
          type="text"
          onClick={copyMessage}
        />
      </Tooltip>
    </div>
  );
}

async function copyText(text: string) {
  if (typeof document !== 'undefined' && copyTextWithSelection(text)) {
    return;
  }
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    try {
      await withTimeout(navigator.clipboard.writeText(text), 300);
      return;
    } catch {
      // Embedded webviews can expose Clipboard API but reject writes.
    }
  }
  if (typeof document === 'undefined') {
    throw new Error('clipboard is unavailable');
  }
  copyTextWithSelection(text);
}

function copyTextWithSelection(text: string) {
  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.setAttribute('readonly', '');
  textarea.style.position = 'fixed';
  textarea.style.top = '-1000px';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  textarea.setSelectionRange(0, textarea.value.length);
  try {
    return document.execCommand('copy');
  } finally {
    document.body.removeChild(textarea);
  }
}

function withTimeout<T>(promise: Promise<T>, timeoutMs: number) {
  return new Promise<T>((resolve, reject) => {
    const timer = window.setTimeout(() => reject(new Error('clipboard write timed out')), timeoutMs);
    promise.then(
      (value) => {
        window.clearTimeout(timer);
        resolve(value);
      },
      (error: unknown) => {
        window.clearTimeout(timer);
        reject(error);
      },
    );
  });
}

function formatMessageTime(createdAt?: number) {
  if (!createdAt) {
    return '';
  }
  const milliseconds = normalizeTimestamp(createdAt);
  return new Intl.DateTimeFormat('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(milliseconds));
}

function progressLabel(status?: string) {
  switch (status) {
    case 'waiting_permission':
      return '等待权限确认';
    case 'running':
    case 'queued':
      return '正在处理';
    case 'cancelled':
      return '已取消';
    case 'failed':
      return '执行失败';
    default:
      return status || '正在处理';
  }
}

function permissionStatusLabel(status?: string) {
  switch (status) {
    case 'pending':
      return '等待权限确认';
    case 'allowed':
    case 'allowed_once':
      return '已允许';
    case 'allowed_session':
      return '本会话已允许';
    case 'denied':
      return '已拒绝';
    case 'cancelled':
      return '已取消';
    case 'expired':
      return '已过期';
    default:
      return status || '权限记录';
  }
}

function progressDetail(item: ConversationTimelineItemViewModel) {
  if (item.status !== 'failed' && item.status !== 'cancelled') {
    return '';
  }
  return item.error || item.summary || '';
}

function formatMissingArtifacts(paths: string[]) {
  if (paths.length === 1) {
    return paths[0];
  }
  return `${paths.length} 个文件缺失：${paths.join('、')}`;
}

function formatDuration(startedAt: number, finishedAt: number) {
  const elapsed = Math.max(0, normalizeTimestamp(finishedAt) - normalizeTimestamp(startedAt));
  if (elapsed < 1000) {
    return '<1s';
  }
  if (elapsed < 60_000) {
    return `${Math.round(elapsed / 1000)}s`;
  }
  return `${Math.floor(elapsed / 60_000)}m ${Math.round((elapsed % 60_000) / 1000)}s`;
}

function normalizeTimestamp(value: number) {
  return value < 1_000_000_000_000 ? value * 1000 : value;
}

function minDefined(left?: number, right?: number) {
  if (left === undefined) {
    return right;
  }
  if (right === undefined) {
    return left;
  }
  return Math.min(left, right);
}

function maxDefined(left?: number, right?: number) {
  if (left === undefined) {
    return right;
  }
  if (right === undefined) {
    return left;
  }
  return Math.max(left, right);
}

function summarizeProcessNote(content: string) {
  const normalized = content.replace(/\s+/g, ' ').trim();
  if (normalized.length <= 140) {
    return normalized;
  }
  return `${normalized.slice(0, 140)}...`;
}
