import type { CanonicalTurnProjection } from './canonicalConversationSelectors.ts';
import type { CanonicalAgentTask, CanonicalMessage, CanonicalTodoPlan, CanonicalToolCall, CanonicalToolResult, CanonicalTurn } from './canonicalConversationTypes.ts';
import type { AgentTaskViewModel } from './workbenchTypes.ts';

const sessionId = 'fixture-session';
const meta = (id: string, turnId: string, sequence: number) => ({ id, sessionId, turnId, activitySequence: String(sequence), revision: '1', createdAt: sequence, updatedAt: sequence });
const turn = (id: string, status = 'completed'): CanonicalTurn => ({ ...meta(id, id, 1), status, userMessageId: `${id}-user`, finalMessageId: status === 'completed' ? `${id}-final` : undefined });
const message = (id: string, turnId: string, phase: 'reasoning' | 'final', sequence: number): CanonicalMessage => ({ ...meta(id, turnId, sequence), role: 'assistant', phase, status: 'completed', content: id });
const tool = (id: string, turnId: string, sequence: number, status = 'completed'): CanonicalToolCall => ({ ...meta(id, turnId, sequence), name: 'shell', source: 'runtime', kind: 'command', status, assistantStepId: `${turnId}-step` });
const result = (id: string, turnId: string, sequence: number, toolCallId: string, status = 'completed'): CanonicalToolResult => ({ ...meta(id, turnId, sequence), toolCallId, ordinal: 0, status });
const task = (id: string, turnId: string, sequence: number, status: string, teamId?: string): CanonicalAgentTask => ({ ...meta(id, turnId, sequence), status, progress: status === 'completed' ? 100 : 40, title: id, teamId, teamRole: teamId ? id : undefined });
const taskView = (entity: CanonicalAgentTask): AgentTaskViewModel => ({ id: entity.id, parentSessionId: entity.sessionId, parentTurnId: entity.turnId, teamId: entity.teamId, title: entity.title ?? entity.id, role: entity.teamRole, kind: entity.teamId ? 'agent_team' : 'subagent', status: entity.status, progress: entity.progress ?? 0, startedAt: entity.createdAt, updatedAt: entity.updatedAt });
const projection = (entity: CanonicalTurn, process: CanonicalTurnProjection['process'], todoPlan?: CanonicalTodoPlan): CanonicalTurnProjection => ({ turn: entity, final: entity.finalMessageId ? message(entity.finalMessageId, entity.id, 'final', 99) : undefined, process, todoPlan });

const noToolTurn = turn('turn-no-tool');
const multiToolTurn = turn('turn-multi-tool');
const failedToolTurn = turn('turn-failed-tool', 'failed');
const todoTurn = turn('turn-todo', 'running');
const subagentTurn = turn('turn-subagent', 'running');
const teamTurn = turn('turn-team', 'running');
const failedCall = tool('tool-failed', failedToolTurn.id, 2, 'failed');
const todoPlan: CanonicalTodoPlan = { ...meta('todo-plan', todoTurn.id, 2), ownerTurnId: todoTurn.id, status: 'active', items: [{ id: 'todo-1', order: 0, status: 'in_progress', content: 'Phase 0', activeForm: 'Implementing Phase 0' }] };
const subagent = task('agent-solo', subagentTurn.id, 2, 'running');
const teamMemberA = task('agent-team-a', teamTurn.id, 2, 'running', 'team-alpha');
const teamMemberB = task('agent-team-b', teamTurn.id, 3, 'completed', 'team-alpha');

export const conversationPresentationFixtures = {
  noTool: { projection: projection(noToolTurn, [message('reasoning-no-tool', noToolTurn.id, 'reasoning', 2)]), tasks: [] },
  multiTool: { projection: projection(multiToolTurn, [tool('tool-a', multiToolTurn.id, 2), result('result-a', multiToolTurn.id, 3, 'tool-a'), tool('tool-b', multiToolTurn.id, 4), result('result-b', multiToolTurn.id, 5, 'tool-b')]), tasks: [] },
  failedTool: { projection: projection(failedToolTurn, [failedCall, result('result-failed', failedToolTurn.id, 3, failedCall.id, 'failed')]), tasks: [] },
  todo: { projection: projection(todoTurn, [], todoPlan), tasks: [] },
  subagent: { projection: projection(subagentTurn, [subagent]), tasks: [taskView(subagent)] },
  team: { projection: projection(teamTurn, [teamMemberA, teamMemberB]), tasks: [taskView(teamMemberA), taskView(teamMemberB)] },
} satisfies Record<string, { projection: CanonicalTurnProjection; tasks: AgentTaskViewModel[] }>;

export const conversationPresentationFixtureSessionId = sessionId;
