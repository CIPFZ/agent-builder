import type { CanonicalConversationStore } from './canonicalConversationStore.ts';
import type { CanonicalAgentTask, CanonicalPermission, CanonicalTodoPlan } from './canonicalConversationTypes.ts';
import type { AgentTaskViewModel, PermissionRequestViewModel, TodoSummaryViewModel } from './workbenchTypes.ts';

export interface CanonicalStructuredActivity {
  todoPlansByTurnId: Record<string, TodoSummaryViewModel>;
  activeTodo?: TodoSummaryViewModel;
  permissionsById: Record<string, PermissionRequestViewModel>;
  pendingPermissions: PermissionRequestViewModel[];
  agentTasksById: Record<string, AgentTaskViewModel>;
  agentTasks: AgentTaskViewModel[];
  agentTeams: Record<string, AgentTaskViewModel[]>;
}

export function selectCanonicalStructuredActivity(store?: CanonicalConversationStore): CanonicalStructuredActivity {
  const empty: CanonicalStructuredActivity = { todoPlansByTurnId: {}, permissionsById: {}, pendingPermissions: [], agentTasksById: {}, agentTasks: [], agentTeams: {} };
  if (!store?.sessionId) return empty;
  const plans = Object.values(store.todoPlansById).filter((plan) => plan.sessionId === store.sessionId).sort(compareActivity);
  for (const plan of plans) empty.todoPlansByTurnId[plan.ownerTurnId] = projectTodo(plan);
  const active = [...plans].reverse().map(projectTodo).find((plan) => plan.total > 0 && plan.inProgress + plan.pending > 0);
  empty.activeTodo = active ?? (plans.length > 0 ? projectTodo(plans[plans.length - 1]) : undefined);
  for (const permission of Object.values(store.permissionsById).filter((item) => item.sessionId === store.sessionId).sort(compareActivity)) {
    const projected = projectPermission(permission, store);
    empty.permissionsById[permission.id] = projected;
    if (isPendingPermission(permission.status)) empty.pendingPermissions.push(projected);
  }
  for (const task of Object.values(store.agentTasksById).filter((item) => item.sessionId === store.sessionId).sort(compareActivity)) {
    const projected = projectAgentTask(task);
    empty.agentTasksById[task.id] = projected;
    empty.agentTasks.push(projected);
    if (task.teamId) (empty.agentTeams[task.teamId] ??= []).push(projected);
  }
  return empty;
}

function projectTodo(plan: CanonicalTodoPlan): TodoSummaryViewModel {
  const items = [...plan.items].sort((left, right) => left.order - right.order).map((item) => ({ id: item.id, content: item.content, status: item.status, activeForm: item.activeForm, updatedAt: plan.updatedAt }));
  return { sessionId: plan.sessionId, turnId: plan.ownerTurnId, items, pending: items.filter((item) => item.status === 'pending').length, inProgress: items.filter((item) => item.status === 'in_progress').length, completed: items.filter((item) => item.status === 'completed').length, total: items.length, updatedAt: plan.updatedAt };
}

function projectPermission(permission: CanonicalPermission, store: CanonicalConversationStore): PermissionRequestViewModel {
  const tool = store.toolCallsById[permission.toolCallId];
  const target = tool?.targets?.[0];
  return { id: permission.id, sessionId: permission.sessionId, turnId: permission.turnId, toolCallId: permission.toolCallId, toolName: tool?.name ?? 'tool', description: permission.description || tool?.command || tool?.inputJson, action: permission.action ?? tool?.name ?? 'execute', risk: permission.risk ?? tool?.risk, status: permission.status, path: permission.path || target, target: permission.target || target, reason: permission.reason || permission.policyReason, policyMode: permission.policyMode, policyReason: permission.policyReason, policyRuleId: permission.policyRuleId, policyRuleSource: permission.policyRuleSource, policyScopeKind: permission.policyScopeKind, policyScopeValue: permission.policyScopeValue, policyTargetSummary: permission.policyTargetSummary || target || tool?.workingDir, createdAt: permission.requestedAt || permission.createdAt, decidedAt: permission.decidedAt };
}

function projectAgentTask(task: CanonicalAgentTask): AgentTaskViewModel {
  return { id: task.id, parentSessionId: task.sessionId, parentTurnId: task.turnId, parentToolCallId: task.parentToolCallId, parentTaskId: task.parentTaskId, childSessionId: task.childSessionId, teamId: task.teamId, dependencies: task.dependencies, title: task.title || task.teamRole || 'Agent task', kind: task.teamId ? 'agent_team' : task.kind || 'subagent', role: task.teamRole, name: task.name, promptSummary: task.promptSummary, model: task.model, provider: task.provider, allowedTools: task.allowedTools, capabilityScope: task.capabilityScope, cwd: task.cwd, worktree: task.worktree, status: task.status, progress: task.progress ?? 0, resultSummary: task.resultSummary, artifactRefs: task.artifactRefs, outputRefs: task.outputRefs, messages: task.messages?.map((message) => ({ id: message.id, taskId: task.id, direction: message.direction, kind: message.kind, status: message.status, sequence: message.sequence, contentSummary: message.contentSummary, relatedToolCallId: message.relatedToolCallId, relatedMessageId: message.relatedMessageId, artifactRefs: message.artifactRefs, createdAt: message.createdAt, deliveredAt: message.deliveredAt, processedAt: message.processedAt, error: message.error })), messageCount: task.messageCount, messagesTruncated: task.messagesTruncated, startedAt: task.startedAt || task.createdAt, updatedAt: task.updatedAt, finishedAt: task.finishedAt, error: task.error };
}

function compareActivity(left: { activitySequence: string; id: string }, right: { activitySequence: string; id: string }) { const a = BigInt(left.activitySequence || '0'); const b = BigInt(right.activitySequence || '0'); return a < b ? -1 : a > b ? 1 : left.id.localeCompare(right.id); }
function isPendingPermission(status: string) { return status === 'pending' || status === 'requested' || status === 'waiting' || status === 'waiting_permission'; }
