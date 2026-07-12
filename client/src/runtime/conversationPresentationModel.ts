import { groupCanonicalProcess, type CanonicalPresentationItem } from './canonicalConversationPresentation.ts';
import type { CanonicalTurnProjection } from './canonicalConversationSelectors.ts';
import type { CanonicalPermission, CanonicalToolCall } from './canonicalConversationTypes.ts';
import type { AgentTaskViewModel } from './workbenchTypes.ts';

export type ProcessAttentionState = 'none' | 'active' | 'waiting' | 'failed';

export interface ProcessDisclosureModel {
  id: string;
  turnId: string;
  status: string;
  hasFinalResponse: boolean;
  attention: ProcessAttentionState;
  toolCount: number;
  failedToolCount: number;
  activeToolCount: number;
  agentCount: number;
  failedAgentCount: number;
  activeAgentCount: number;
  orderedItems: CanonicalPresentationItem[];
}

export interface AgentActivitySummary {
  id: string;
  sessionId: string;
  total: number;
  active: number;
  waiting: number;
  completed: number;
  failed: number;
  attention: ProcessAttentionState;
}

export interface AgentTeamPresentation {
  id: string;
  teamId: string;
  sessionId: string;
  memberIds: string[];
  members: AgentTaskViewModel[];
  summary: AgentActivitySummary;
}

export function deriveProcessDisclosureModel(projection: CanonicalTurnProjection, agentTasks: AgentTaskViewModel[] = []): ProcessDisclosureModel {
  const tools = projection.process.filter(isToolCall);
  const agents = agentTasks.filter((task) => task.parentTurnId === projection.turn.id);
  const permissions = projection.process.filter(isPermission);
  const failedToolCount = tools.filter((tool) => isFailed(tool.status)).length;
  const activeToolCount = tools.filter((tool) => isActive(tool.status)).length;
  const failedAgentCount = agents.filter((task) => isFailed(task.status)).length;
  const activeAgentCount = agents.filter((task) => isActive(task.status)).length;
  const waiting = permissions.some((permission) => isWaiting(permission.status)) || agents.some((task) => isWaiting(task.status));
  const attention: ProcessAttentionState = waiting ? 'waiting' : failedToolCount + failedAgentCount > 0 ? 'failed' : activeToolCount + activeAgentCount > 0 || isActive(projection.turn.status) ? 'active' : 'none';
  return {
    id: `process:${projection.turn.id}`,
    turnId: projection.turn.id,
    status: projection.turn.status,
    hasFinalResponse: Boolean(projection.final),
    attention,
    toolCount: tools.length,
    failedToolCount,
    activeToolCount,
    agentCount: agents.length,
    failedAgentCount,
    activeAgentCount,
    orderedItems: groupCanonicalProcess(projection.process),
  };
}

export function deriveAgentActivitySummary(sessionId: string, tasks: AgentTaskViewModel[]): AgentActivitySummary {
  const owned = tasks.filter((task) => task.parentSessionId === sessionId);
  const active = owned.filter((task) => isActive(task.status)).length;
  const waiting = owned.filter((task) => isWaiting(task.status)).length;
  const failed = owned.filter((task) => isFailed(task.status)).length;
  const completed = owned.filter((task) => isCompleted(task.status)).length;
  return { id: `agent-activity:${sessionId}`, sessionId, total: owned.length, active, waiting, completed, failed, attention: waiting ? 'waiting' : failed > 0 ? 'failed' : active > 0 ? 'active' : 'none' };
}

export function deriveAgentTeamPresentations(sessionId: string, tasks: AgentTaskViewModel[]): AgentTeamPresentation[] {
  const teams = new Map<string, AgentTaskViewModel[]>();
  for (const task of tasks) if (task.parentSessionId === sessionId && task.teamId) (teams.get(task.teamId) ?? (teams.set(task.teamId, []), teams.get(task.teamId)!)).push(task);
  return [...teams.entries()].map(([teamId, members]) => {
    return { id: `agent-team:${teamId}`, teamId, sessionId, memberIds: members.map((task) => task.id), members, summary: deriveAgentActivitySummary(sessionId, members) };
  });
}

function isToolCall(entity: CanonicalTurnProjection['process'][number]): entity is CanonicalToolCall { return 'name' in entity && 'source' in entity; }
function isPermission(entity: CanonicalTurnProjection['process'][number]): entity is CanonicalPermission { return 'toolCallId' in entity && !('ordinal' in entity); }
function isActive(status: string) { return ['queued', 'running', 'streaming', 'in_progress', 'starting'].includes(status); }
function isWaiting(status: string) { return ['pending', 'requested', 'waiting', 'waiting_permission', 'blocked'].includes(status); }
function isFailed(status: string) { return ['failed', 'error', 'interrupted', 'cancelled', 'canceled'].includes(status); }
function isCompleted(status: string) { return ['completed', 'complete', 'succeeded', 'success', 'done'].includes(status); }
