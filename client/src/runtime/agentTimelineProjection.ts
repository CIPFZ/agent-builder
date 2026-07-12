import type { AgentTaskViewModel, ConversationTimelineItemViewModel } from './workbenchTypes.ts';
import { deriveAgentTeamPresentations } from './conversationPresentationModel.ts';

export function projectAgentTimeline(items: ConversationTimelineItemViewModel[]): ConversationTimelineItemViewModel[] {
  const membersByTeam = new Map<string, AgentTaskViewModel[]>();
  for (const item of items) {
    const task = item.kind === 'agent_task' ? item.agentTask : undefined;
    if (task?.teamId) (membersByTeam.get(task.teamId) ?? (membersByTeam.set(task.teamId, []), membersByTeam.get(task.teamId)!)).push(task);
  }
  const sessionId = items.find((item) => item.agentTask)?.agentTask?.parentSessionId ?? '';
  const presentations = new Map(deriveAgentTeamPresentations(sessionId, [...membersByTeam.values()].flat()).map((team) => [team.teamId, team]));
  const emittedTeams = new Set<string>();
  const projected: ConversationTimelineItemViewModel[] = [];
  for (const item of items) {
    const task = item.kind === 'agent_task' ? item.agentTask : undefined;
    if (!task?.teamId) {
      projected.push(item);
      continue;
    }
    if (emittedTeams.has(task.teamId)) continue;
    emittedTeams.add(task.teamId);
    const presentation = presentations.get(task.teamId);
    const members = presentation?.members ?? membersByTeam.get(task.teamId) ?? [task];
    projected.push({
      id: presentation?.id ?? `agent-team:${task.teamId}`,
      kind: 'agent_team',
      sessionId: item.sessionId,
      turnId: item.turnId,
      teamId: task.teamId,
      title: 'Agent Team',
      status: aggregateTeamStatus(members),
      createdAt: item.createdAt,
      updatedAt: Math.max(...members.map((member) => member.updatedAt ?? member.startedAt ?? 0)),
      agentTasks: members,
    });
  }
  return projected;
}

function aggregateTeamStatus(members: AgentTaskViewModel[]) {
  if (members.some((task) => isWaiting(task.status))) return 'waiting';
  if (members.some((task) => isFailed(task.status))) return 'failed';
  if (members.some((task) => isActive(task.status))) return 'running';
  return members.every((task) => isCompleted(task.status)) ? 'completed' : 'idle';
}

export function isWaitingAgentTaskStatus(status?: string) { return isWaiting(status); }
export function isFailedAgentTaskStatus(status?: string) { return isFailed(status); }
function isWaiting(status?: string) { return status === 'waiting' || status === 'blocked' || status === 'waiting_permission' || status === 'pending'; }
function isFailed(status?: string) { return status === 'failed' || status === 'interrupted' || status === 'cancelled' || status === 'canceled' || status === 'error'; }
function isActive(status?: string) { return status === 'queued' || status === 'running' || status === 'streaming' || status === 'in_progress' || status === 'starting'; }
function isCompleted(status?: string) { return status === 'completed' || status === 'complete' || status === 'success' || status === 'succeeded' || status === 'done'; }
