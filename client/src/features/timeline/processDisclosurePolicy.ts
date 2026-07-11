export interface ProcessDisclosureState {
  status?: string;
  explorationStatus?: string;
  itemStatuses: Array<string | undefined>;
}

export function shouldAutoOpenProcess(state: ProcessDisclosureState) {
  if (isTerminalProcessStatus(state.status)) return false;
  if (isActiveProcessStatus(state.status)) return true;
  if (state.explorationStatus === 'exploring') return true;
  return state.itemStatuses.some((status) => isActiveProcessStatus(status));
}

export function isActiveProcessStatus(status?: string) {
  return status === 'running' || status === 'queued' || status === 'waiting_permission' || status === 'streaming';
}

export function isFailedProcessStatus(status?: string) {
  return status === 'failed' || status === 'denied' || status === 'cancelled' || status === 'interrupted';
}

export function isTerminalProcessStatus(status?: string) {
  return status === 'completed' || status === 'success' || status === 'failed' || status === 'denied' || status === 'cancelled' || status === 'interrupted';
}
