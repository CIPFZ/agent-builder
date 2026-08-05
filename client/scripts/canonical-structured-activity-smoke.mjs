import assert from 'node:assert/strict';
import { hydrateCanonicalConversationStore } from '../src/runtime/canonicalConversationStore.ts';
import { selectCanonicalStructuredActivity } from '../src/runtime/canonicalStructuredActivity.ts';
import { selectCanonicalConversationTurnViewModels } from '../src/runtime/canonicalConversationView.ts';

const meta = (id, seq, turnId = 'turn-1') => ({ id, sessionId: 'session-1', turnId, activitySequence: seq, revision: seq, createdAt: Number(seq), updatedAt: Number(seq) });
const snapshot = { schemaVersion: 2, sessionId: 'session-1', cursor: '20', scope: 'full', turns: [{ ...meta('turn-1', '1'), status: 'running' }], messages: [], assistantSteps: [], toolCalls: [{ ...meta('tool-1', '2'), name: 'shell', source: 'builtin', status: 'waiting_permission', commandPreview: 'rm file', targets: ['/work/file'] }], toolResults: [], permissions: [{ ...meta('permission-1', '3'), toolCallId: 'tool-1', status: 'pending', action: 'execute', risk: 'high', reason: 'destructive' }], todoPlans: [{ ...meta('plan-1', '4'), ownerTurnId: 'turn-1', status: 'active', items: [{ id: 'todo-1', order: 0, status: 'in_progress', content: 'Implement', activeForm: 'Implementing' }] }], agentTasks: [{ ...meta('task-1', '5'), parentToolCallId: 'tool-1', teamId: 'team-1', teamRole: 'reviewer', title: 'Review', status: 'running', progress: 40, resultRefs: ['artifact://one'], messages: [{ id: 'task-message-1', direction: 'child_to_parent', kind: 'progress', status: 'delivered', sequence: 1, contentSummary: 'halfway' }], messageCount: 65, messagesTruncated: true }, { ...meta('task-2', '6'), parentTaskId: 'task-1', teamId: 'team-1', teamRole: 'tester', title: 'Test', status: 'completed', progress: 100 }], notices: [{ ...meta('notice:hook:one', '7'), kind: 'hook', status: 'completed', summary: 'checked' }, { ...meta('notice:compact:one', '8'), kind: 'compact', status: 'completed', summary: 'compacted', dataJson: '{"trigger":"auto","pre_tokens":1000,"post_tokens":200}' }, { ...meta('notice:recovery:one', '9'), kind: 'recovery', status: 'failed', summary: 'retry failed' }, { ...meta('notice:context:agents', '10'), kind: 'context', status: 'completed', summary: 'runtime_context_load', dataJson: '{"source_id":"project:/work/AGENTS.md"}' }] };
let store = hydrateCanonicalConversationStore(snapshot);
const structured = selectCanonicalStructuredActivity(store);
assert.equal(structured.activeTodo.turnId, 'turn-1', 'Todo owner comes from ownerTurnId');
assert.equal(structured.activeTodo.items[0].activeForm, 'Implementing');
assert.equal(structured.pendingPermissions[0].id, 'permission-1');
assert.equal(structured.pendingPermissions[0].toolName, 'shell');
assert.equal(structured.pendingPermissions[0].target, '/work/file');
assert.equal(structured.agentTeams['team-1'].length, 2, 'Agent Team groups only by explicit teamId');
assert.equal(structured.agentTasksById['task-1'].messages[0].contentSummary, 'halfway', 'late task messages remain visible in canonical detail');
assert.equal(structured.agentTasksById['task-1'].messagesTruncated, true);

const turn = selectCanonicalConversationTurnViewModels(store, structured)[0];
const permissionRow = turn.process.items.find((item) => item.kind === 'permission');
const teamRow = turn.process.items.find((item) => item.kind === 'agent_team');
assert.equal(turn.process.items.find((item) => item.kind === 'tool_group').status, 'waiting_permission', 'tool group preserves its permission-waiting lifecycle');
assert.equal(permissionRow.permission, structured.permissionsById['permission-1'], 'timeline and gate share one Permission projection');
assert.deepEqual(teamRow.agentTasks, [structured.agentTasksById['task-1'], structured.agentTasksById['task-2']], 'Team timeline and detail share the ordered canonical AgentTask projections');
assert.equal(turn.process.items.find((item) => item.kind === 'tool_group').toolCalls[0].agentTask, structured.agentTasksById['task-1'], 'tool detail shares the canonical AgentTask projection');
assert.equal(turn.process.items.filter((item) => item.kind === 'hook_run').length, 1);
assert.equal(turn.process.items.filter((item) => item.kind === 'compact_boundary').length, 1);
assert.equal(turn.process.items.filter((item) => item.kind === 'recovery_notice').length, 1);
assert.equal(turn.process.items.filter((item) => item.kind === 'context_source').length, 0, 'prompt assembly context stays in diagnostics rather than the conversation timeline');
assert.equal(turn.process.items.some((item) => item.kind === 'todo_summary'), false, 'Todo remains a capsule rather than a duplicate timeline summary');

store = hydrateCanonicalConversationStore({ ...snapshot, cursor: '21', scope: 'window', turns: [], messages: [], toolCalls: [], permissions: [], todoPlans: [], agentTasks: [], notices: [], window: { turnIds: [] } }, store);
assert.equal(selectCanonicalStructuredActivity(store).activeTodo.turnId, 'turn-1', 'window omission cannot reassign Todo ownership');
console.log('canonical structured activity smoke passed');
