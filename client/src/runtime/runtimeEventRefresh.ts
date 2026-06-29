import type { RuntimeEventViewModel } from './workbenchTypes.ts';

const immediateRefreshEvents = new Set([
  'turn.started',
  'turn.completed',
  'turn.failed',
  'turn.cancelled',
  'turn.interrupted',
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
]);

export const runtimeEventImmediateRefreshDelayMS = 0;
export const runtimeEventCoalescedRefreshDelayMS = 350;

export function runtimeEventRefreshDelay(event: RuntimeEventViewModel) {
  const type = event.type || '';
  if (immediateRefreshEvents.has(type) || type.includes('diagnostic') || type.includes('artifact')) {
    return runtimeEventImmediateRefreshDelayMS;
  }
  if (coalescedRefreshEvents.has(type) || type.includes('delta') || type.includes('token') || type.includes('progress')) {
    return runtimeEventCoalescedRefreshDelayMS;
  }
  return 180;
}

export function nextRuntimeEventSequence(current: number, event: RuntimeEventViewModel) {
  return typeof event.sequence === 'number' && Number.isFinite(event.sequence) && event.sequence > current ? event.sequence : current;
}
