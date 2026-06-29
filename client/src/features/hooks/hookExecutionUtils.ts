export function executionStatusColor(status?: string) {
  switch (status) {
    case 'completed':
      return 'green';
    case 'blocked':
    case 'denied':
      return 'orange';
    case 'failed':
      return 'red';
    case 'started':
    case 'running':
      return 'blue';
    case 'skipped':
      return 'default';
    default:
      return 'default';
  }
}
