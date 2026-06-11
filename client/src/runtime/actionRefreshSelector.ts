export type RuntimeActionRefreshTarget =
  | 'status'
  | 'turn_activity'
  | 'session_activity_window'
  | 'session_activity'
  | 'tool_calls'
  | 'run'
  | 'run_projection'
  | 'run_transition_history'
  | 'diagnostics'
  | 'permissions'
  | 'mcp_requests'
  | 'run_scheduler_plan';

interface RuntimeWriteActionMetadataDTO {
  accepted?: boolean;
  refreshTargets?: unknown;
}

export interface RuntimeWriteActionResponseDTO {
  accepted?: boolean;
  action?: RuntimeWriteActionMetadataDTO;
  refreshTargets?: unknown;
}

const runtimeActionRefreshTargetSet = new Set<RuntimeActionRefreshTarget>([
  'status',
  'turn_activity',
  'session_activity_window',
  'session_activity',
  'tool_calls',
  'run',
  'run_projection',
  'run_transition_history',
  'diagnostics',
  'permissions',
  'mcp_requests',
  'run_scheduler_plan',
]);

export function runtimeActionRefreshTargets(response: unknown): RuntimeActionRefreshTarget[] | undefined {
  if (!isRecord(response)) {
    return undefined;
  }
  const action = isRecord(response.action) ? response.action : undefined;
  if (action) {
    if (action.accepted !== true) {
      return undefined;
    }
    return normalizeRuntimeActionRefreshTargets(action.refreshTargets);
  }
  if (response.accepted === false) {
    return undefined;
  }
  return normalizeRuntimeActionRefreshTargets(response.refreshTargets);
}

function normalizeRuntimeActionRefreshTargets(value: unknown): RuntimeActionRefreshTarget[] | undefined {
  if (!Array.isArray(value) || value.length === 0) {
    return undefined;
  }
  const result: RuntimeActionRefreshTarget[] = [];
  for (const item of value) {
    if (typeof item !== 'string' || !runtimeActionRefreshTargetSet.has(item as RuntimeActionRefreshTarget)) {
      return undefined;
    }
    const target = item as RuntimeActionRefreshTarget;
    if (!result.includes(target)) {
      result.push(target);
    }
  }
  return result.length > 0 ? result : undefined;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}
