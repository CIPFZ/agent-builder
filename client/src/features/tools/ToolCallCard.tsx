import {
  CheckOutlined,
  CloseCircleOutlined,
  CodeOutlined,
  CopyOutlined,
  EditOutlined,
  FileTextOutlined,
  LoadingOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import { Button, Collapse, Tag, Typography, message } from 'antd';
import type { ToolCallViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './ToolCallCard.module.css';

type ToolKind = 'file_read' | 'file_write' | 'file_edit' | 'file_search' | 'shell' | 'generic';

export function ToolCallCard({ toolCall, toolCalls }: { toolCall?: ToolCallViewModel; toolCalls?: ToolCallViewModel[] }) {
  const calls = toolCalls?.length ? toolCalls : toolCall ? [toolCall] : [];
  if (calls.length === 0) {
    return null;
  }
  if (calls.length > 1) {
    return <ToolCallGroup toolCalls={calls} />;
  }
  return <SingleToolCallCard toolCall={calls[0]} />;
}

function SingleToolCallCard({ toolCall }: { toolCall: ToolCallViewModel }) {
  const [messageApi, messageContextHolder] = message.useMessage();
  const kind = toolKind(toolCall);
  const output = readableToolOutput(toolCall);
  const detail = toolDetail(toolCall);
  const hasDetails = hasToolDetails(toolCall, detail, output);
  const defaultActiveKey = shouldOpenByDefault([toolCall]) ? ['details'] : [];

  return (
    <section className={styles.process} data-testid="tool-call-card" data-tool-call-id={toolCall.id} data-tool-kind={kind} data-tool-status={toolCall.status}>
      {messageContextHolder}
      <Collapse
        ghost
        size="small"
        defaultActiveKey={defaultActiveKey}
        expandIconPlacement="end"
        items={[
          {
            key: 'details',
            showArrow: hasDetails,
            collapsible: hasDetails ? undefined : 'disabled',
            label: (
              <div className={styles.summary}>
                <span className={styles.summaryTitle}>
                  <ToolIcon kind={kind} status={toolCall.status} />
                  {toolCall.display?.title || summaryTitle(toolCall, kind)}
                </span>
                <span className={styles.summaryMeta}>
                  {toolDuration(toolCall)}
                  <ToolStatus status={toolCall.status} />
                </span>
              </div>
            ),
            children: hasDetails ? (
              <ToolDetails toolCall={toolCall} kind={kind} output={output} detail={detail} onCopy={async (text) => copyText(text, messageApi)} />
            ) : null,
          },
        ]}
      />
    </section>
  );
}

function ToolCallGroup({ toolCalls }: { toolCalls: ToolCallViewModel[] }) {
  const [messageApi, messageContextHolder] = message.useMessage();
  const kinds = toolCalls.map((call) => toolKind(call));
  const groupKind = new Set(kinds).size === 1 ? kinds[0] : 'generic';
  const status = groupStatus(toolCalls);
  const startedAt = minTimestamp(toolCalls.map((call) => call.startedAt));
  const finishedAt = maxTimestamp(toolCalls.map((call) => call.finishedAt));
  const defaultActiveKey = shouldOpenByDefault(toolCalls) ? ['details'] : [];

  return (
    <section className={styles.process} data-testid="tool-call-card" data-tool-call-id={toolCalls.map((call) => call.id).join(',')} data-tool-kind={groupKind} data-tool-status={status}>
      {messageContextHolder}
      <Collapse
        ghost
        size="small"
        defaultActiveKey={defaultActiveKey}
        expandIconPlacement="end"
        items={[
          {
            key: 'details',
            showArrow: true,
            label: (
              <div className={styles.summary}>
                <span className={styles.summaryTitle}>
                  <ToolIcon kind={groupKind} status={status} />
                  {groupSummaryTitle(toolCalls, groupKind)}
                </span>
                <span className={styles.summaryMeta}>
                  {startedAt && finishedAt && <span>{formatDuration(startedAt, finishedAt)}</span>}
                  <ToolStatus status={status} />
                </span>
              </div>
            ),
            children: (
              <div className={styles.groupDetails}>
                {toolCalls.map((call, index) => (
                  <ToolDetails
                    key={call.id}
                    toolCall={call}
                    kind={toolKind(call)}
                    output={readableToolOutput(call)}
                    detail={toolDetail(call)}
                    title={toolDetailTitle(call, index)}
                    onCopy={async (text) => copyText(text, messageApi)}
                  />
                ))}
              </div>
            ),
          },
        ]}
      />
    </section>
  );
}

function ToolDetails({
  detail,
  kind,
  onCopy,
  output,
  title,
  toolCall,
}: {
  detail?: string;
  kind: ToolKind;
  onCopy: (text: string) => Promise<void>;
  output: string;
  title?: string;
  toolCall: ToolCallViewModel;
}) {
  const shellLike = kind === 'shell';
  const detailsText = [detail, output, toolCall.stderr, toolCall.error].filter(Boolean).join('\n\n');

  return (
    <div className={styles.details}>
      {detail && (
        <div className={shellLike ? styles.shellPanel : styles.detailPanel}>
          <div className={styles.panelHeader}>
            <span>{title || (shellLike ? 'Shell' : detailLabel(kind))}</span>
            <Button aria-label="复制工具详情" className={styles.copyButton} icon={<CopyOutlined />} size="small" type="text" onClick={() => void onCopy(detailsText || detail)} />
          </div>
          <Typography.Paragraph className={shellLike ? styles.commandLine : styles.detailText} copyable={false}>
            {detail}
          </Typography.Paragraph>
          {output ? <pre className={styles.output}>{output}</pre> : shellLike && toolCall.status === 'completed' ? <div className={styles.emptyOutput}>无输出</div> : null}
          {toolCall.stderr && <pre className={`${styles.output} ${styles.errorOutput}`}>{toolCall.stderr}</pre>}
          {toolCall.error && <pre className={`${styles.output} ${styles.errorOutput}`}>{toolCall.error}</pre>}
        </div>
      )}

      {!detail && output && (
        <div className={styles.detailPanel}>
          <div className={styles.panelHeader}>
            <span>{title || '输出'}</span>
            <Button aria-label="复制工具输出" className={styles.copyButton} icon={<CopyOutlined />} size="small" type="text" onClick={() => void onCopy(output)} />
          </div>
          <pre className={styles.output}>{output}</pre>
        </div>
      )}

      {(toolCall.risk || toolCall.policyMode || toolCall.policyReason || toolCall.policyTargetSummary || toolExitCode(toolCall) !== undefined || toolRefCount(toolCall) > 0) && (
        <div className={styles.meta}>
          {toolExitCode(toolCall) !== undefined && <Tag>exit {toolExitCode(toolCall)}</Tag>}
          {toolCall.outputRefs?.length ? <Tag>{toolCall.outputRefs.length} output refs</Tag> : null}
          {toolCall.artifactRefs?.length ? <Tag>{toolCall.artifactRefs.length} artifacts</Tag> : null}
          {toolCall.diffRefs?.length ? <Tag>{toolCall.diffRefs.length} diffs</Tag> : null}
          {toolCall.risk && <Tag icon={<SafetyCertificateOutlined />}>{riskLabel(toolCall.risk)}</Tag>}
          {toolCall.policyMode && <Tag>{policyModeLabel(toolCall.policyMode)}</Tag>}
          {toolCall.policyReason && <Tag>{toolCall.policyReason}</Tag>}
          {toolCall.policyTargetSummary && <Tag>{toolCall.policyTargetSummary}</Tag>}
        </div>
      )}
    </div>
  );
}

function ToolIcon({ kind, status }: { kind: ToolKind; status: string }) {
  if (status === 'running' || status === 'queued' || status === 'waiting_permission') {
    return <LoadingOutlined spin />;
  }
  if (status === 'failed' || status === 'denied' || status === 'cancelled') {
    return <CloseCircleOutlined />;
  }
  switch (kind) {
    case 'file_read':
      return <FileTextOutlined />;
    case 'file_write':
    case 'file_edit':
      return <EditOutlined />;
    case 'file_search':
      return <SearchOutlined />;
    case 'shell':
      return <CodeOutlined />;
    default:
      return <CodeOutlined />;
  }
}

function ToolStatus({ status }: { status: string }) {
  if (status === 'completed') {
    return (
      <span className={styles.successStatus}>
        <CheckOutlined />
        成功
      </span>
    );
  }
  if (status === 'failed' || status === 'denied' || status === 'cancelled') {
    return <Tag color="error">{statusLabel(status)}</Tag>;
  }
  return <Tag color={status === 'waiting_permission' ? 'warning' : 'processing'}>{statusLabel(status)}</Tag>;
}

function summaryTitle(toolCall: ToolCallViewModel, kind: ToolKind) {
  const name = toolCall.name || 'tool';
  if (kind === 'shell') {
    if (toolCall.status === 'running' || toolCall.status === 'queued') {
      return '正在运行 1 条命令';
    }
    if (toolCall.status === 'waiting_permission') {
      return '等待运行 1 条命令';
    }
    return '已运行 1 条命令';
  }
  if (kind === 'file_read') {
    return toolCall.status === 'completed' ? '已读取文件' : '读取文件';
  }
  if (kind === 'file_write') {
    return toolCall.status === 'completed' ? '已写入文件' : '写入文件';
  }
  if (kind === 'file_edit') {
    return toolCall.status === 'completed' ? '已编辑文件' : '编辑文件';
  }
  if (kind === 'file_search') {
    return toolCall.status === 'completed' ? '已搜索文件' : '搜索文件';
  }
  if (toolCall.status === 'completed') {
    return `已运行 ${name}`;
  }
  if (toolCall.status === 'running' || toolCall.status === 'queued') {
    return `正在运行 ${name}`;
  }
  return name;
}

function groupSummaryTitle(toolCalls: ToolCallViewModel[], kind: ToolKind) {
  const count = toolCalls.length;
  const status = groupStatus(toolCalls);
  const prefix = status === 'running' || status === 'queued' ? '正在运行' : status === 'waiting_permission' ? '等待运行' : '已运行';
  if (kind === 'shell') {
    return `${prefix} ${count} 条命令`;
  }
  if (kind === 'file_read') {
    return `${status === 'completed' ? '已读取' : '读取'} ${count} 个文件`;
  }
  if (kind === 'file_write') {
    return `${status === 'completed' ? '已写入' : '写入'} ${count} 个文件`;
  }
  if (kind === 'file_edit') {
    return `${status === 'completed' ? '已编辑' : '编辑'} ${count} 个文件`;
  }
  if (kind === 'file_search') {
    return `${status === 'completed' ? '已搜索' : '搜索'} ${count} 次`;
  }
  return `${prefix} ${count} 个工具`;
}

function toolDetailTitle(toolCall: ToolCallViewModel, index: number) {
  const kind = toolKind(toolCall);
  if (kind === 'shell') {
    return `Shell ${index + 1}`;
  }
  return `${detailLabel(kind)} ${index + 1}`;
}

function detailLabel(kind: ToolKind) {
  switch (kind) {
    case 'file_read':
      return '读取';
    case 'file_write':
      return '写入';
    case 'file_edit':
      return '编辑';
    case 'file_search':
      return '搜索';
    default:
      return '详情';
  }
}

function toolKind(toolCall: ToolCallViewModel): ToolKind {
  if (isToolKind(toolCall.display?.kind)) {
    return toolCall.display.kind;
  }
  const name = toolCall.name.toLowerCase();
  const summary = `${toolCall.inputSummary ?? ''} ${toolCall.command ?? ''}`.toLowerCase();
  if (isShellTool(toolCall)) {
    return 'shell';
  }
  if (name.includes('edit') || name.includes('patch') || summary.includes('apply_patch')) {
    return 'file_edit';
  }
  if (name.includes('write') || name.includes('create') || summary.includes('write')) {
    return 'file_write';
  }
  if (name.includes('read') || name.includes('view') || name.includes('open') || summary.includes('read')) {
    return 'file_read';
  }
  if (isSearchToolName(name) || summary.includes('glob') || summary.includes('grep') || summary.includes('search')) {
    return 'file_search';
  }
  return 'generic';
}

function isToolKind(value?: string): value is ToolKind {
  return value === 'file_read' || value === 'file_write' || value === 'file_edit' || value === 'file_search' || value === 'shell' || value === 'generic';
}

function isSearchToolName(name: string) {
  return name === 'glob' || name === 'grep' || name === 'list' || name === 'ls' || name === 'dir' || name.includes('search') || name.includes('find');
}

function toolDetail(toolCall: ToolCallViewModel) {
  return toolCall.display?.detail || toolCall.display?.target || toolCall.display?.command || toolCall.command || toolCall.inputSummary;
}

function toolExitCode(toolCall: ToolCallViewModel) {
  return toolCall.display?.exitCode ?? toolCall.exitCode;
}

function toolRefCount(toolCall: ToolCallViewModel) {
  return (toolCall.outputRefs?.length ?? 0) + (toolCall.artifactRefs?.length ?? 0) + (toolCall.diffRefs?.length ?? 0);
}

function toolDuration(toolCall: ToolCallViewModel) {
  if (toolCall.display?.durationMs !== undefined) {
    return <span>{formatDurationMs(toolCall.display.durationMs)}</span>;
  }
  if (toolCall.finishedAt && toolCall.startedAt) {
    return <span>{formatDuration(toolCall.startedAt, toolCall.finishedAt)}</span>;
  }
  return null;
}

function isShellTool(toolCall: ToolCallViewModel) {
  const name = toolCall.name.toLowerCase();
  const source = toolCall.source?.toLowerCase() ?? '';
  const commonCommands = new Set(['bash', 'cmd', 'command', 'go', 'npm', 'node', 'python', 'powershell', 'pwsh', 'shell']);
  return Boolean(toolCall.command || toolCall.risk === 'execute' || source.includes('shell') || commonCommands.has(name) || name.includes('command'));
}

function hasToolDetails(toolCall: ToolCallViewModel, detail?: string, output?: string) {
  return Boolean(detail || output || toolCall.stdout || toolCall.stderr || toolCall.error || toolCall.policyReason || toolCall.policyTargetSummary);
}

function shouldOpenByDefault(toolCalls: ToolCallViewModel[]) {
  return toolCalls.some((call) => call.status === 'failed' || call.status === 'running' || call.status === 'queued');
}

function groupStatus(toolCalls: ToolCallViewModel[]) {
  if (toolCalls.some((call) => call.status === 'failed' || call.status === 'denied' || call.status === 'cancelled')) {
    return 'failed';
  }
  if (toolCalls.some((call) => call.status === 'waiting_permission')) {
    return 'waiting_permission';
  }
  if (toolCalls.some((call) => call.status === 'running')) {
    return 'running';
  }
  if (toolCalls.some((call) => call.status === 'queued')) {
    return 'queued';
  }
  if (toolCalls.every((call) => call.status === 'completed')) {
    return 'completed';
  }
  return toolCalls[toolCalls.length - 1]?.status ?? 'completed';
}

function readableToolOutput(toolCall: ToolCallViewModel) {
  const output = extractWrappedOutput(toolCall.stdout) || extractWrappedOutput(toolCall.outputSummary);
  if (output) {
    return output.trim();
  }
  if (toolCall.status === 'failed' || toolCall.status === 'denied' || toolCall.status === 'cancelled') {
    return (extractWrappedOutput(toolCall.stderr) || toolCall.error || '').trim();
  }
  return '';
}

function extractWrappedOutput(value?: string) {
  const text = value?.trim();
  if (!text) {
    return '';
  }

  const parsed = parseJSON(text);
  if (parsed && typeof parsed === 'object') {
    const record = parsed as Record<string, unknown>;
    for (const key of ['output', 'stdout', 'stderr', 'content', 'data']) {
      const field = record[key];
      if (typeof field === 'string' && field.trim()) {
        return field.trim();
      }
    }
  }

  return text;
}

function parseJSON(value: string): unknown {
  try {
    return JSON.parse(value);
  } catch {
    return undefined;
  }
}

function statusLabel(status: string) {
  switch (status) {
    case 'waiting_permission':
      return '等待权限';
    case 'running':
      return '执行中';
    case 'queued':
      return '排队中';
    case 'denied':
      return '已拒绝';
    case 'cancelled':
      return '已取消';
    case 'failed':
      return '失败';
    default:
      return status;
  }
}

function riskLabel(risk: string) {
  switch (risk) {
    case 'read':
      return '只读';
    case 'write':
      return '写入';
    case 'execute':
      return '执行';
    case 'network':
      return '网络';
    case 'secret':
      return '敏感';
    case 'destructive':
      return '破坏性';
    default:
      return risk;
  }
}

function policyModeLabel(mode: string) {
  switch (mode) {
    case 'ask':
      return '请求批准';
    case 'auto_read':
      return '替我审批';
    case 'full_access':
      return '完全访问';
    case 'plan':
      return '计划模式';
    case 'deny_all':
      return '全部阻止';
    default:
      return mode;
  }
}

function formatDuration(startedAt: number, finishedAt: number) {
  const elapsed = Math.max(0, normalizeTimestamp(finishedAt) - normalizeTimestamp(startedAt));
  return formatDurationMs(elapsed);
}

function formatDurationMs(elapsed: number) {
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

function minTimestamp(values: Array<number | undefined>) {
  const timestamps = values.filter((value): value is number => typeof value === 'number');
  return timestamps.length ? Math.min(...timestamps) : undefined;
}

function maxTimestamp(values: Array<number | undefined>) {
  const timestamps = values.filter((value): value is number => typeof value === 'number');
  return timestamps.length ? Math.max(...timestamps) : undefined;
}

async function copyText(text: string, messageApi: ReturnType<typeof message.useMessage>[0]) {
  if (!text.trim()) {
    return;
  }
  try {
    await copyToClipboard(text);
    void messageApi.success('已复制');
  } catch {
    void messageApi.error('复制失败');
  }
}

async function copyToClipboard(text: string) {
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
