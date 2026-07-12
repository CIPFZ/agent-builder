import assert from 'node:assert/strict';
import { conversationPresentationFixtures as fixtures, conversationPresentationFixtureSessionId as sessionId } from '../src/runtime/conversationPresentationFixtures.ts';
import { deriveAgentActivitySummary, deriveAgentTeamPresentations, deriveProcessDisclosureModel } from '../src/runtime/conversationPresentationModel.ts';

const noTool = deriveProcessDisclosureModel(fixtures.noTool.projection);
assert.equal(noTool.id, 'process:turn-no-tool');
assert.equal(noTool.toolCount, 0);
assert.equal(noTool.attention, 'none');

const multiTool = deriveProcessDisclosureModel(fixtures.multiTool.projection);
assert.equal(multiTool.toolCount, 2);
assert.equal(multiTool.orderedItems[0].key, 'tool-group:turn-multi-tool:turn-multi-tool-step:command');
assert.deepEqual(multiTool.orderedItems.map((item) => item.key), ['tool-group:turn-multi-tool:turn-multi-tool-step:command', 'toolResult:result-a', 'toolResult:result-b']);

const failed = deriveProcessDisclosureModel(fixtures.failedTool.projection);
assert.equal(failed.failedToolCount, 1);
assert.equal(failed.attention, 'failed');

const todo = deriveProcessDisclosureModel(fixtures.todo.projection);
assert.equal(todo.orderedItems.length, 0, 'Todo remains outside the process sequence');
assert.equal(fixtures.todo.projection.todoPlan?.id, 'todo-plan');

const solo = deriveAgentActivitySummary(sessionId, fixtures.subagent.tasks);
assert.deepEqual({ id: solo.id, total: solo.total, active: solo.active, attention: solo.attention }, { id: `agent-activity:${sessionId}`, total: 1, active: 1, attention: 'active' });
assert.equal(deriveProcessDisclosureModel(fixtures.subagent.projection, fixtures.subagent.tasks).agentCount, 1);

const teams = deriveAgentTeamPresentations(sessionId, fixtures.team.tasks);
assert.equal(teams.length, 1);
assert.equal(teams[0].id, 'agent-team:team-alpha');
assert.deepEqual(teams[0].memberIds, ['agent-team-a', 'agent-team-b']);
assert.deepEqual({ total: teams[0].summary.total, active: teams[0].summary.active, completed: teams[0].summary.completed }, { total: 2, active: 1, completed: 1 });

console.log('conversation presentation model smoke passed');
