export interface ProcessDisclosureState {
  status?: string;
  explorationStatus?: string;
  failedCount?: number;
  itemStatuses: Array<string | undefined>;
}

export function shouldAutoOpenProcess(state: ProcessDisclosureState) {
  if (state.explorationStatus === 'exploring' || (state.failedCount ?? 0) > 0) return true;
  if (isFailedStatus(state.status)) return true;
  return isActiveStatus(state.status) || state.itemStatuses.some((status) => isActiveStatus(status) || isFailedStatus(status));
}

export function isActiveProcessStatus(status?: string) {
  return status === 'running' || status === 'queued' || status === 'waiting_permission' || status === 'streaming';
}

export function isFailedProcessStatus(status?: string) {
  return status === 'failed' || status === 'denied' || status === 'cancelled' || status === 'interrupted';
}

const isActiveStatus = isActiveProcessStatus;
const isFailedStatus = isFailedProcessStatus;
