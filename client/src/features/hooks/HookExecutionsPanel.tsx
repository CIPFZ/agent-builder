import { BranchesOutlined, FilterOutlined } from '@ant-design/icons';
import { Checkbox, Empty, Flex, Select, Tag } from 'antd';
import { useMemo, useState } from 'react';
import type { HookExecutionSummaryViewModel, HookExecutionViewModel } from '../../runtime/workbenchTypes.ts';
import { HookExecutionDetailDrawer } from './HookExecutionDetailDrawer.tsx';
import { executionStatusColor } from './hookExecutionUtils.ts';
import styles from './HookExecutionsPanel.module.css';

export function HookExecutionsPanel({
  summary,
  onLoadExecution,
}: {
  summary?: HookExecutionSummaryViewModel;
  onLoadExecution?: (executionId: string) => Promise<HookExecutionViewModel>;
}) {
  const [eventFilter, setEventFilter] = useState<string>();
  const [statusFilter, setStatusFilter] = useState<string>();
  const [onlyProblem, setOnlyProblem] = useState(false);
  const [onlyChanged, setOnlyChanged] = useState(false);
  const [selected, setSelected] = useState<HookExecutionViewModel | undefined>();
  const items = useMemo(() => summary?.items ?? [], [summary?.items]);
  const events = useMemo(() => [...new Set(items.map((item) => item.event).filter(Boolean))], [items]);
  const statuses = useMemo(() => [...new Set(items.map((item) => item.status).filter(Boolean))], [items]);
  const filtered = useMemo(
    () =>
      items.filter((item) => {
        if (eventFilter && item.event !== eventFilter) {
          return false;
        }
        if (statusFilter && item.status !== statusFilter) {
          return false;
        }
        if (onlyProblem && !isProblemExecution(item)) {
          return false;
        }
        if (onlyChanged && !item.inputRewritten && !item.contextInjected) {
          return false;
        }
        return true;
      }),
    [eventFilter, items, onlyChanged, onlyProblem, statusFilter],
  );

  if (!summary) {
    return (
      <aside className={styles.panel} aria-label="Hook executions">
        <Header summary={summary} />
        <Empty className={styles.empty} image={Empty.PRESENTED_IMAGE_SIMPLE} description="Hook execution API unavailable" />
      </aside>
    );
  }

  return (
    <aside className={styles.panel} aria-label="Hook executions" data-testid="hook-executions-panel">
      <Header summary={summary} />
      <div className={styles.filters}>
        <Select
          allowClear
          placeholder="Event"
          size="small"
          value={eventFilter}
          options={events.map((event) => ({ label: event, value: event }))}
          onChange={setEventFilter}
        />
        <Select
          allowClear
          placeholder="Status"
          size="small"
          value={statusFilter}
          options={statuses.map((status) => ({ label: status, value: status }))}
          onChange={setStatusFilter}
        />
        <Checkbox checked={onlyProblem} onChange={(event) => setOnlyProblem(event.target.checked)}>
          blocked/failed
        </Checkbox>
        <Checkbox checked={onlyChanged} onChange={(event) => setOnlyChanged(event.target.checked)}>
          rewritten/context
        </Checkbox>
      </div>

      {filtered.length === 0 ? (
        <Empty className={styles.empty} image={Empty.PRESENTED_IMAGE_SIMPLE} description={items.length === 0 ? 'No hook executions for current session' : 'No matching hook executions'} />
      ) : (
        <div className={styles.list}>
          {filtered.map((execution) => (
            <ExecutionRow key={execution.id} execution={execution} onOpen={setSelected} />
          ))}
        </div>
      )}

      <HookExecutionDetailDrawer
        executionId={selected?.id}
        fallback={selected}
        open={Boolean(selected)}
        onClose={() => setSelected(undefined)}
        onLoad={onLoadExecution}
      />
    </aside>
  );
}

function Header({ summary }: { summary?: HookExecutionSummaryViewModel }) {
  return (
    <div className={styles.header}>
      <span className={styles.heading}>
        <BranchesOutlined />
        <span>Hook executions</span>
      </span>
      <Flex wrap gap={4} justify="flex-end">
        <Tag icon={<FilterOutlined />}>{summary?.total ?? 0}</Tag>
        {summary?.blocked ? <Tag color="orange">blocked {summary.blocked}</Tag> : null}
        {summary?.failed ? <Tag color="red">failed {summary.failed}</Tag> : null}
        {summary?.rewritten ? <Tag color="purple">rewritten {summary.rewritten}</Tag> : null}
        {summary?.contextInjected ? <Tag color="blue">context {summary.contextInjected}</Tag> : null}
      </Flex>
    </div>
  );
}

function ExecutionRow({ execution, onOpen }: { execution: HookExecutionViewModel; onOpen: (execution: HookExecutionViewModel) => void }) {
  return (
    <button className={styles.row} type="button" onClick={() => onOpen(execution)}>
      <div className={styles.rowHeader}>
        <span className={styles.rowTitle}>{execution.hookName || execution.hookId || execution.event}</span>
        <Tag color={executionStatusColor(execution.status)}>{execution.status}</Tag>
      </div>
      <div className={styles.rowMeta}>
        <span>{formatTime(execution.startedAt)}</span>
        <span>{formatDuration(execution.durationMs)}</span>
        <Tag>{execution.event}</Tag>
        {execution.toolCallId ? <span>tool {shortID(execution.toolCallId)}</span> : null}
        {execution.turnId ? <span>turn {shortID(execution.turnId)}</span> : null}
        {execution.taskId ? <span>task {shortID(execution.taskId)}</span> : null}
      </div>
      {execution.reason ? <div className={styles.reason}>{execution.reason}</div> : null}
      {execution.error ? <div className={styles.error}>{execution.error}</div> : null}
      <div className={styles.flags}>
        {execution.inputRewritten ? <Tag color="purple">input rewritten</Tag> : null}
        {execution.contextInjected ? <Tag color="blue">context injected</Tag> : null}
        {execution.redacted ? <Tag color="gold">redacted</Tag> : null}
      </div>
    </button>
  );
}

function isProblemExecution(execution: HookExecutionViewModel) {
  return execution.status === 'blocked' || execution.status === 'denied' || execution.status === 'failed' || Boolean(execution.error);
}

function shortID(value: string) {
  return value.length <= 10 ? value : value.slice(0, 10);
}

function formatTime(value?: number) {
  if (!value) {
    return 'unknown time';
  }
  return new Intl.DateTimeFormat(undefined, {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date(normalizeTimestamp(value)));
}

function formatDuration(value?: number) {
  if (!value) {
    return '0ms';
  }
  if (value < 1000) {
    return `${value}ms`;
  }
  return `${Math.round(value / 100) / 10}s`;
}

function normalizeTimestamp(value: number) {
  return value < 1_000_000_000_000 ? value * 1000 : value;
}
