import type { RuntimeEventViewModel } from './workbenchTypes.ts';

const immediateRefreshEvents = new Set([
  'session.selection.cleared',
  'turn.started',
  'turn.completed',
  'turn.failed',
  'turn.cancelled',
  'turn.interrupted',
  'recovery.status.changed',
  'recovery.turn.resumed',
  'recovery.turn.discarded',
  'recovery.error.classified',
  'recovery.retry.started',
  'recovery.retry.completed',
  'recovery.retry.failed',
  'recovery.history_hygiene.applied',
  'recovery.context.compact_retry_started',
  'recovery.context.compact_retry_completed',
  'recovery.context.compact_retry_failed',
  'tool.call.started',
  'tool.call.completed',
  'tool.call.failed',
  'tool.call.cancelled',
  'permission.requested',
  'permission.policy.applied',
  'permission.decided',
  'artifact.ref.created',
  'task.artifact.created',
  'tool.output.ref.created',
  'output.ref.created',
  'snapshot.required',
  'hook.discovered',
  'hook.configured',
  'hook.execution.started',
  'hook.execution.completed',
  'hook.execution.skipped',
  'hook.execution.blocked',
  'hook.execution.failed',
  'hook.context.injected',
  'hook.input.rewritten',
]);

const coalescedRefreshEvents = new Set([
  'message.created',
  'message.updated',
  'message.completed',
  'turn.progress',
  'tool.call.output',
  'task.progress',
  'task.message.created',
  'task.message.delivered',
  'task.message.processed',
  'usage.updated',
  'context.usage.updated',
  'compact.started',
  'compact.completed',
  'compact.failed',
]);

const backgroundRefreshEvents = new Set([
  'session.created',
  'session.updated',
  'session.deleted',
  'task.started',
  'task.completed',
  'task.failed',
  'task.cancelled',
  'task.interrupted',
  'task.role.loaded',
  'task.result.updated',
]);

const backgroundRefreshPrefixes = [
  'project.',
  'model.',
  'provider.',
  'skill.discovery.',
  'plugin.',
  'mcp.server.',
];

export const runtimeEventImmediateRefreshDelayMS = 0;
export const runtimeEventCoalescedRefreshDelayMS = 350;

export function runtimeEventRefreshDelay(event: RuntimeEventViewModel) {
  const type = event.type || '';
  if (!type) return undefined;
  if (immediateRefreshEvents.has(type) || type.includes('diagnostic') || type.includes('artifact')) {
    return runtimeEventImmediateRefreshDelayMS;
  }
  if (coalescedRefreshEvents.has(type) || type.includes('delta') || type.includes('token') || type.includes('progress')) {
    return runtimeEventCoalescedRefreshDelayMS;
  }
  if (backgroundRefreshEvents.has(type) || backgroundRefreshPrefixes.some((prefix) => type.startsWith(prefix))) {
    return 180;
  }
  // Runtime events are durable audit/invalidation facts, not a command to
  // rebuild the entire Workbench. Unknown events stay observable in the
  // stream but must opt in above before they can allocate a full hydration.
  return undefined;
}

// runtimeEventCoveredByOutputStream reports whether the event is one that
// the per-session output stream already delivers as a materialized batch.
// When the stream is available, the shell skips the full-workbench refresh
// path for these types to avoid duplicate work — status, usage, and
// recovery events still need the classic refresh loop.
export function runtimeEventCoveredByOutputStream(event: RuntimeEventViewModel) {
  const type = event.type || '';
  if (!type) return false;
  return (
    type.startsWith('message.') ||
    type.startsWith('tool.call.') ||
    type === 'output.text.delta' ||
    type === 'permission.requested' ||
    type === 'permission.decided'
  );
}

export function nextRuntimeEventSequence(current: number, event: RuntimeEventViewModel) {
  return typeof event.sequence === 'number' && Number.isFinite(event.sequence) && event.sequence > current ? event.sequence : current;
}
