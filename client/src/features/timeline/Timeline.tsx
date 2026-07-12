import { useState } from 'react';
import { message } from 'antd';
import type { ConversationTimelineItemViewModel, HookExecutionSummaryViewModel, HookExecutionViewModel } from '../../runtime/workbenchTypes.ts';
import type { ConversationTurnViewModel } from '../../runtime/conversation/conversationTypes.ts';
import { HookExecutionDetailDrawer } from '../hooks/HookExecutionDetailDrawer.tsx';
import { HookTimelineRow } from '../hooks/HookTimelineRow.tsx';
import { TimelineMessage } from './TimelineMessage.tsx';
import { CompactTraceRow, ContextGovernanceRow, TurnDiagnosticWarning, WorkflowNoticeRow } from './ProcessNoticeItems.tsx';
import { AgentTaskTimelineRow, PermissionTraceRow } from './InteractiveProcessItems.tsx';
import { ToolTraceGroup } from './ToolProcessItems.tsx';
import { ProcessNarration } from './ProcessNarration.tsx';
import type { RenderTimelineItem } from './processGrouping.ts';
import { ProcessDisclosure } from './ProcessDisclosure.tsx';
import styles from './Timeline.module.css';

interface TimelineProps {
  turns: ConversationTurnViewModel[];
  hookExecutions?: HookExecutionSummaryViewModel;
  onAgentTaskOpen?: (taskID: string) => void;
  onHookExecutionLoad?: (executionID: string) => Promise<HookExecutionViewModel>;
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

export function Timeline({ turns, hookExecutions, onAgentTaskOpen, onHookExecutionLoad }: TimelineProps) {
  const [messageApi, messageContextHolder] = message.useMessage();
  const [selectedHookExecution, setSelectedHookExecution] = useState<HookExecutionViewModel | undefined>();
  const blocks: TimelineTurnBlock[] = turns.map((turn) => ({
    id: turn.id,
    turnId: turn.id,
    userMessage: turn.user,
    explorationSummary: turn.process.exploration ? {
      id: `exploration:${turn.id}`,
      kind: 'exploration_summary',
      turnId: turn.id,
      status: turn.process.exploration.status,
      exploration: turn.process.exploration,
    } : undefined,
    processItems: turn.process.items,
    finalMessage: turn.final,
    looseItems: [],
    status: turn.status,
    startedAt: turn.startedAt,
    finishedAt: turn.finishedAt,
  }));

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
      {block.processItems.length > 0 && (
        <ProcessDisclosure
          turnId={block.turnId}
          status={block.status}
          startedAt={block.startedAt}
          finishedAt={block.finishedAt}
          exploration={block.explorationSummary?.exploration}
          items={block.processItems}
          renderItem={(item) => <TimelineProcessItem hookExecutions={hookExecutions} item={item} onAgentTaskOpen={onAgentTaskOpen} onHookOpen={onHookOpen} />}
        />
      )}
      {block.finalMessage && <TimelineMessage item={block.finalMessage} messageApi={messageApi} />}
      {block.looseItems.map((item) => (
        <TimelineProcessItem key={item.id} hookExecutions={hookExecutions} item={item} onAgentTaskOpen={onAgentTaskOpen} onHookOpen={onHookOpen} />
      ))}
    </section>
  );
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
    return <ToolTraceGroup toolCalls={item.toolCalls} onAgentTaskOpen={onAgentTaskOpen} />;
  }
  if (item.kind === 'tool_call_group') {
    return (
      <>
        <ToolTraceGroup toolCalls={item.toolCalls} onAgentTaskOpen={onAgentTaskOpen} />
        {item.toolCalls.flatMap((toolCall) => highSignalHooks(hookExecutions, item.turnId, toolCall.id)).map((execution) => (
          <HookTimelineRow key={execution.id} execution={execution} onOpen={onHookOpen} />
        ))}
      </>
    );
  }
  if (item.kind === 'tool_group' && item.toolCalls) {
    return <ToolTraceGroup toolCalls={item.toolCalls} onAgentTaskOpen={onAgentTaskOpen} />;
  }
  if ((item.kind === 'tool_call' || item.kind === 'tool_group') && item.toolCall) {
    return (
      <>
        <ToolTraceGroup toolCalls={[item.toolCall]} onAgentTaskOpen={onAgentTaskOpen} />
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
    return <ProcessNarration item={item} />;
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
    return <CompactTraceRow item={item} />;
  }
  if (isContextGovernanceItem(item)) {
    return <ContextGovernanceRow item={item} />;
  }
  if (item.kind === 'agent_task') {
    return item.agentTask ? <AgentTaskTimelineRow item={item} onAgentTaskOpen={onAgentTaskOpen} /> : <WorkflowNoticeRow item={item} />;
  }
  if (item.kind === 'message' || item.kind === 'assistant_message') {
    return <ProcessNarration item={item} />;
  }
  return null;
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

function isContextGovernanceItem(item: ConversationTimelineItemViewModel) {
	return item.kind === 'context_source';
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

function progressDetail(item: ConversationTimelineItemViewModel) {
  if (item.status !== 'failed' && item.status !== 'cancelled') {
    return '';
  }
  return item.error || item.summary || '';
}
