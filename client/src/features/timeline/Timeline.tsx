import { memo, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { message } from 'antd';
import type { ConversationTimelineItemViewModel, HookExecutionSummaryViewModel, HookExecutionViewModel } from '../../runtime/workbenchTypes.ts';
import type { ConversationTurnViewModel } from '../../runtime/conversation/conversationTypes.ts';
import { HookExecutionDetailDrawer } from '../hooks/HookExecutionDetailDrawer.tsx';
import { HookTimelineRow } from '../hooks/HookTimelineRow.tsx';
import { TimelineMessage } from './TimelineMessage.tsx';
import { CompactTraceRow, ContextGovernanceRow, TurnDiagnosticWarning, WorkflowNoticeRow } from './ProcessNoticeItems.tsx';
import { AgentTaskTimelineRow, AgentTeamTimelineRow, PermissionTraceRow } from './InteractiveProcessItems.tsx';
import { ToolTraceGroup } from './ToolProcessItems.tsx';
import { ProcessNarration } from './ProcessNarration.tsx';
import type { RenderTimelineItem } from './processGrouping.ts';
import { ProcessDisclosure } from './ProcessDisclosure.tsx';
import { isActiveProcessStatus, isFailedProcessStatus } from './processDisclosurePolicy.ts';
import styles from './Timeline.module.css';

interface TimelineProps {
  turns: ConversationTurnViewModel[];
  hookExecutions?: HookExecutionSummaryViewModel;
  onAgentTaskOpen?: (taskID: string) => void;
  onHookExecutionLoad?: (executionID: string) => Promise<HookExecutionViewModel>;
  onMessageContentLoad?: (sessionID: string, messageID: string) => Promise<string>;
  onObjectContentLoad?: (refID: string) => Promise<string>;
}

interface TimelineTurnBlock {
  id: string;
  revisionKey: string;
  turnId?: string;
  userMessage?: ConversationTimelineItemViewModel;
  explorationSummary?: ConversationTimelineItemViewModel;
  processItems: ConversationTimelineItemViewModel[];
  finalMessage?: ConversationTimelineItemViewModel;
  looseItems: ConversationTimelineItemViewModel[];
  status?: string;
  startedAt?: number;
  finishedAt?: number;
  error?: string;
}

export function Timeline({ turns, hookExecutions, onAgentTaskOpen, onHookExecutionLoad, onMessageContentLoad, onObjectContentLoad }: TimelineProps) {
  const [messageApi, messageContextHolder] = message.useMessage();
  const [selectedHookExecution, setSelectedHookExecution] = useState<HookExecutionViewModel | undefined>();
  const blocks: TimelineTurnBlock[] = turns.map((turn) => ({
    id: turn.id,
    revisionKey: turn.revisionKey,
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
    error: turn.error,
  }));

  return (
    <div className={styles.timeline} data-testid="conversation-timeline">
      {messageContextHolder}
      {blocks.map((block) => (
        <VirtualizedTurnBlock
          key={block.id}
          block={block}
          hookExecutions={hookExecutions}
          messageApi={messageApi}
          onAgentTaskOpen={onAgentTaskOpen}
          onHookOpen={setSelectedHookExecution}
          onMessageContentLoad={onMessageContentLoad}
          onObjectContentLoad={onObjectContentLoad}
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

const VirtualizedTurnBlock = memo(function VirtualizedTurnBlock({
  block,
  hookExecutions,
  messageApi,
  onAgentTaskOpen,
  onHookOpen,
  onMessageContentLoad,
  onObjectContentLoad,
}: {
  block: TimelineTurnBlock;
  hookExecutions?: HookExecutionSummaryViewModel;
  messageApi: ReturnType<typeof message.useMessage>[0];
  onAgentTaskOpen?: (taskID: string) => void;
  onHookOpen?: (execution: HookExecutionViewModel) => void;
  onMessageContentLoad?: (sessionID: string, messageID: string) => Promise<string>;
  onObjectContentLoad?: (refID: string) => Promise<string>;
}) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [nearViewport, setNearViewport] = useState(true);
  const [measuredHeight, setMeasuredHeight] = useState<number>();
  const keepMounted = isActiveTurnStatus(block.status);
  const mounted = keepMounted || nearViewport || measuredHeight === undefined || typeof IntersectionObserver === 'undefined';

  useLayoutEffect(() => {
    const node = containerRef.current;
    if (!node || !mounted) return;
    // Active Turns are always mounted, so retaining a virtualization height
    // while every token changes layout only creates redundant observer/state
    // work. Start measuring once the Turn settles and can leave the viewport.
    if (keepMounted) return;
    const measure = () => {
      const height = node.getBoundingClientRect().height;
      if (height > 0) setMeasuredHeight((current) => current === height ? current : height);
    };
    measure();
    if (typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(measure);
    observer.observe(node);
    return () => observer.disconnect();
  }, [keepMounted, mounted]);

  useEffect(() => {
    const node = containerRef.current;
    if (!node || typeof IntersectionObserver === 'undefined') return;
    const observer = new IntersectionObserver(([entry]) => setNearViewport(entry.isIntersecting), { rootMargin: '1000px 0px' });
    observer.observe(node);
    return () => observer.disconnect();
  }, []);

  return (
    <div
      ref={containerRef}
      className={styles.turnViewport}
      data-turn-mounted={mounted}
      style={!mounted && measuredHeight !== undefined ? { height: measuredHeight } : undefined}
    >
      {mounted && <TurnBlock block={block} hookExecutions={hookExecutions} messageApi={messageApi} onAgentTaskOpen={onAgentTaskOpen} onHookOpen={onHookOpen} onMessageContentLoad={onMessageContentLoad} onObjectContentLoad={onObjectContentLoad} />}
    </div>
  );
}, (previous, next) => (
  previous.block.revisionKey === next.block.revisionKey &&
  previous.block.status === next.block.status &&
  previous.hookExecutions === next.hookExecutions &&
  previous.messageApi === next.messageApi &&
  previous.onAgentTaskOpen === next.onAgentTaskOpen &&
  previous.onHookOpen === next.onHookOpen
  && previous.onMessageContentLoad === next.onMessageContentLoad
  && previous.onObjectContentLoad === next.onObjectContentLoad
));

function TurnBlock({
  block,
  hookExecutions,
  messageApi,
  onAgentTaskOpen,
  onHookOpen,
  onMessageContentLoad,
  onObjectContentLoad,
}: {
  block: TimelineTurnBlock;
  hookExecutions?: HookExecutionSummaryViewModel;
  messageApi: ReturnType<typeof message.useMessage>[0];
  onAgentTaskOpen?: (taskID: string) => void;
  onHookOpen?: (execution: HookExecutionViewModel) => void;
  onMessageContentLoad?: (sessionID: string, messageID: string) => Promise<string>;
  onObjectContentLoad?: (refID: string) => Promise<string>;
}) {
  const promptHooks = block.processItems.some((item) => item.kind === 'hook_run') ? [] : highSignalHooks(hookExecutions, block.turnId).filter((execution) => !execution.toolCallId);
  return (
    <section className={styles.turnBlock} data-testid="timeline-turn-block" data-turn-id={block.turnId}>
      {block.userMessage && <TimelineMessage item={block.userMessage} messageApi={messageApi} onContentLoad={onMessageContentLoad} />}
      {promptHooks.map((execution) => (
        <HookTimelineRow key={execution.id} execution={execution} onOpen={onHookOpen} />
      ))}
      {shouldRenderProcess(block) && (
        <ProcessDisclosure
          turnId={block.turnId}
          status={block.status}
          startedAt={block.startedAt}
          finishedAt={block.finishedAt}
          error={block.error}
          exploration={block.explorationSummary?.exploration}
          hasFinalResponse={Boolean(block.finalMessage)}
          items={block.processItems}
          renderItem={(item) => <TimelineProcessItem hookExecutions={hookExecutions} item={item} onAgentTaskOpen={onAgentTaskOpen} onHookOpen={onHookOpen} onObjectContentLoad={onObjectContentLoad} />}
        />
      )}
      {block.finalMessage && <TimelineMessage item={block.finalMessage} messageApi={messageApi} onContentLoad={onMessageContentLoad} />}
      {block.looseItems.map((item) => (
        <TimelineProcessItem key={item.id} hookExecutions={hookExecutions} item={item} onAgentTaskOpen={onAgentTaskOpen} onHookOpen={onHookOpen} onObjectContentLoad={onObjectContentLoad} />
      ))}
    </section>
  );
}

function TimelineProcessItem({
  item,
  hookExecutions,
  onAgentTaskOpen,
  onHookOpen,
  onObjectContentLoad,
}: {
  item: RenderTimelineItem;
  hookExecutions?: HookExecutionSummaryViewModel;
  onAgentTaskOpen?: (taskID: string) => void;
  onHookOpen?: (execution: HookExecutionViewModel) => void;
  onObjectContentLoad?: (refID: string) => Promise<string>;
}) {
  if (item.kind === 'tool_call_group') {
    return (
      <>
        <ToolTraceGroup toolCalls={item.toolCalls} onAgentTaskOpen={onAgentTaskOpen} onObjectContentLoad={onObjectContentLoad} />
        {item.toolCalls.flatMap((toolCall) => highSignalHooks(hookExecutions, item.turnId, toolCall.id)).map((execution) => (
          <HookTimelineRow key={execution.id} execution={execution} onOpen={onHookOpen} />
        ))}
      </>
    );
  }
  if (item.kind === 'tool_group' && item.toolCalls) {
    return <ToolTraceGroup toolCalls={item.toolCalls} onAgentTaskOpen={onAgentTaskOpen} onObjectContentLoad={onObjectContentLoad} />;
  }
  if ((item.kind === 'tool_call' || item.kind === 'tool_group') && item.toolCall) {
    return (
      <>
        <ToolTraceGroup toolCalls={[item.toolCall]} onAgentTaskOpen={onAgentTaskOpen} onObjectContentLoad={onObjectContentLoad} />
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
  if (item.kind === 'agent_team') {
    return <AgentTeamTimelineRow item={item} onAgentTaskOpen={onAgentTaskOpen} />;
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

function isActiveTurnStatus(status?: string) {
  return isActiveProcessStatus(status);
}

function shouldRenderProcess(block: TimelineTurnBlock) {
  return block.processItems.length > 0 || isActiveProcessStatus(block.status) || isFailedProcessStatus(block.status) || (block.status === 'completed' && !block.finalMessage);
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
