import type { RunCheckpointViewModel } from '../../runtime/workbenchTypes.ts';

export function selectResumableCheckpoint(checkpoints?: RunCheckpointViewModel[]) {
  return checkpoints?.find((checkpoint) => checkpoint.resumeEligible);
}
