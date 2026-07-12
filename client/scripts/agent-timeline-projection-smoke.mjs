import assert from 'node:assert/strict';
import fs from 'node:fs';
import { projectAgentTimeline } from '../src/runtime/agentTimelineProjection.ts';

const task = (id, status, teamId, startedAt) => ({ id, parentSessionId: 'session-1', parentTurnId: 'turn-1', title: id, kind: teamId ? 'agent_team' : 'subagent', teamId, status, progress: 0, startedAt, updatedAt: startedAt });
const row = (agentTask) => ({ id: `agentTask:${agentTask.id}`, kind: 'agent_task', turnId: agentTask.parentTurnId, status: agentTask.status, createdAt: agentTask.startedAt, updatedAt: agentTask.updatedAt, agentTask });

const first = projectAgentTimeline([
  row(task('solo', 'waiting', undefined, 1)),
  row(task('member-b', 'running', 'team-1', 2)),
  row(task('member-a', 'completed', 'team-1', 3)),
]);
assert.deepEqual(first.map((item) => item.id), ['agentTask:solo', 'agent-team:team-1']);
assert.equal(first[0].status, 'waiting', 'waiting independent task remains visible');
assert.equal(first[1].status, 'running');
assert.deepEqual(first[1].agentTasks.map((member) => member.id), ['member-b', 'member-a'], 'Team members preserve canonical input order');

const revised = projectAgentTimeline([
  row(task('solo', 'failed', undefined, 1)),
  row(task('member-b', 'completed', 'team-1', 2)),
  row(task('member-a', 'failed', 'team-1', 3)),
]);
assert.deepEqual(revised.map((item) => item.id), first.map((item) => item.id), 'status revisions retain one stable task/Team identity');
assert.equal(revised.filter((item) => item.id === 'agent-team:team-1').length, 1, 'member revisions never append duplicate Team rows');
assert.equal(revised[0].status, 'failed');
assert.equal(revised[1].status, 'failed');

const rowSource = fs.readFileSync(new URL('../src/features/timeline/InteractiveProcessItems.tsx', import.meta.url), 'utf8');
const workspaceSource = fs.readFileSync(new URL('../src/features/workspace/Workspace.tsx', import.meta.url), 'utf8');
assert.match(rowSource, /onRowClick=\{\(\) => onAgentTaskOpen\?\.\(task\.id\)\}/, 'task and Team-member capsules forward canonical task ids');
assert.match(workspaceSource, /setSelectedAgentTaskID\(taskID\);\s*openSingletonPanel\('tasks'\)/, 'timeline task click selects and opens the existing Tasks panel');
assert.doesNotMatch(rowSource, /<Progress\b|InlineExpandable/, 'timeline task capsules do not duplicate detail-panel content');

console.log('agent timeline projection smoke passed');
