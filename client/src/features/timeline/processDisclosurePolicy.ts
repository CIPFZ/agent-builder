export type ProcessDisclosureMode = 'auto' | 'manual_open' | 'manual_closed';

export interface ProcessDisclosureSignal {
  status?: string;
  hasFinalResponse: boolean;
  explorationStatus?: string;
  itemStatuses: Array<string | undefined>;
  hasPendingPermission: boolean;
}

export interface ProcessDisclosureState {
  mode: ProcessDisclosureMode;
  open: boolean;
  completionObserved: boolean;
}

export type ProcessDisclosureAction =
  | { type: 'sync'; signal: ProcessDisclosureSignal }
  | { type: 'manual'; open: boolean };

export function initialProcessDisclosureState(signal: ProcessDisclosureSignal): ProcessDisclosureState {
  const complete = isSafelyCompleteProcess(signal);
  return { mode: 'auto', open: complete ? false : shouldKeepProcessOpen(signal), completionObserved: complete };
}

export function reduceProcessDisclosure(state: ProcessDisclosureState, action: ProcessDisclosureAction): ProcessDisclosureState {
  if (action.type === 'manual') return { ...state, mode: action.open ? 'manual_open' : 'manual_closed', open: action.open };
  const complete = isSafelyCompleteProcess(action.signal);
  // Completion is a lifecycle boundary: collapse once even when the user
  // changed disclosure while the process was still active. A manual choice
  // made after completion remains authoritative because completionObserved is
  // already true by then.
  if (complete && !state.completionObserved) return { mode: 'auto', open: false, completionObserved: true };
  if (state.mode !== 'auto') return state;
  if (shouldKeepProcessOpen(action.signal)) return { ...state, open: true, completionObserved: false };
  return { ...state, completionObserved: state.completionObserved || complete };
}

export function shouldKeepProcessOpen(signal: ProcessDisclosureSignal) {
  if (signal.hasPendingPermission) return true;
  if (isFailedProcessStatus(signal.status)) return true;
  if (isActiveProcessStatus(signal.status)) return true;
  if (isSuccessfulTerminalStatus(signal.status) && !signal.hasFinalResponse) return true;
  if (signal.explorationStatus === 'exploring') return true;
  return signal.itemStatuses.some((status) => isActiveProcessStatus(status) || isFailedProcessStatus(status));
}

export function isSafelyCompleteProcess(signal: ProcessDisclosureSignal) {
  return isSuccessfulTerminalStatus(signal.status) && signal.hasFinalResponse && !signal.hasPendingPermission && !signal.itemStatuses.some((status) => isActiveProcessStatus(status) || isFailedProcessStatus(status));
}

export function isActiveProcessStatus(status?: string) {
  return status === 'running' || status === 'queued' || status === 'waiting' || status === 'blocked' || status === 'waiting_permission' || status === 'streaming' || status === 'in_progress' || status === 'starting';
}

export function isFailedProcessStatus(status?: string) {
  return status === 'failed' || status === 'denied' || status === 'cancelled' || status === 'canceled' || status === 'interrupted' || status === 'error';
}

export function isTerminalProcessStatus(status?: string) {
  return isSuccessfulTerminalStatus(status) || isFailedProcessStatus(status);
}

function isSuccessfulTerminalStatus(status?: string) {
  return status === 'completed' || status === 'complete' || status === 'success' || status === 'succeeded' || status === 'done';
}
