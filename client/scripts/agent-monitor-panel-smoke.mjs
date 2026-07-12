import assert from 'node:assert/strict';
import fs from 'node:fs';
import { projectAgentTaskPanel } from '../src/runtime/agentTaskPanelProjection.ts';

const task = (id, status, teamId) => ({ id, parentSessionId: 'session-1', parentTurnId: 'turn-1', title: id, kind: teamId ? 'agent_team' : 'subagent', teamId, status, progress: 0 });
const tasks = [task('independent', 'running'), task('member-a', 'waiting', 'team-1'), task('member-b', 'completed', 'team-1')];
const projection = projectAgentTaskPanel('session-1', tasks);
assert.deepEqual({ total: projection.summary.total, active: projection.summary.active, waiting: projection.summary.waiting, completed: projection.summary.completed }, { total: 3, active: 1, waiting: 1, completed: 1 });
assert.deepEqual(projection.independent.map((item) => item.id), ['independent']);
assert.equal(projection.teams[0].id, 'agent-team:team-1');
assert.deepEqual(projection.teams[0].memberIds, ['member-a', 'member-b']);
assert.equal(projectAgentTaskPanel('another-session', tasks).summary.total, 0, 'monitor never leaks tasks across Sessions');

const workspace = fs.readFileSync(new URL('../src/features/workspace/Workspace.tsx', import.meta.url), 'utf8');
const list = fs.readFileSync(new URL('../src/features/agentTasks/AgentTaskList.tsx', import.meta.url), 'utf8');
const detail = fs.readFileSync(new URL('../src/features/agentTasks/AgentTaskDetail.tsx', import.meta.url), 'utf8');
const styles = fs.readFileSync(new URL('../src/features/agentTasks/AgentTaskPanel.module.css', import.meta.url), 'utf8');
assert.match(workspace, /AgentActivityMonitor summary=\{agentTaskPresentation\.summary\} onOpen=\{\(\) => openSingletonPanel\('tasks'\)\}/, 'monitor opens the existing Tasks tab without changing selection');
assert.match(workspace, /setSelectedAgentTaskID\(taskID\);\s*openSingletonPanel\('tasks'\)/, 'timeline selection and monitor share the Tasks surface');
assert.match(workspace, /selectedTaskID=\{selectedAgentTaskID\}/, 'controlled selection survives Tasks tab open/close');
assert.match(list, /Agent Team · \$\{team\.teamId\}/);
assert.match(list, /Independent tasks/);
assert.doesNotMatch(detail, /messages\.slice/, 'full canonical bounded message window remains available');
assert.match(detail, /relatedToolCallId/, 'child-tool references remain visible in task details');
assert.match(styles, /\.detail[\s\S]*max-height: min\(70vh, 720px\);[\s\S]*overflow-y: auto;/, 'detail scrolling is locally bounded');

console.log('agent monitor and panel smoke passed');
