import assert from 'node:assert/strict';
import fs from 'node:fs';
import { todoDisplayModel } from '../src/features/todos/todoDisplayPolicy.ts';

const todos = { sessionId: 'session-1', turnId: 'turn-1', pending: 0, inProgress: 0, completed: 2, total: 2, items: [{ id: 'a', content: 'A', status: 'completed' }, { id: 'b', content: 'B', status: 'completed' }] };
assert.equal(todoDisplayModel(todos, 'running').state, 'completed');
assert.equal(todoDisplayModel(todos, 'completed').state, 'completed', 'completed Todo remains as one compact capsule');

const dock = fs.readFileSync(new URL('../src/features/conversationDock/ConversationDock.tsx', import.meta.url), 'utf8');
const dockStyles = fs.readFileSync(new URL('../src/features/conversationDock/ConversationDock.module.css', import.meta.url), 'utf8');
const todo = fs.readFileSync(new URL('../src/features/todos/TodoTaskBar.tsx', import.meta.url), 'utf8');
const todoStyles = fs.readFileSync(new URL('../src/features/todos/TodoTaskBar.module.css', import.meta.url), 'utf8');
const workspace = fs.readFileSync(new URL('../src/features/workspace/Workspace.tsx', import.meta.url), 'utf8');
const permissionStyles = fs.readFileSync(new URL('../src/features/permissions/PermissionGate.module.css', import.meta.url), 'utf8');
const timelineStyles = fs.readFileSync(new URL('../src/features/timeline/Timeline.module.css', import.meta.url), 'utf8');

assert.match(dock, /activeActions\.length[\s\S]*ConversationActions actions=\{activeActions\}[\s\S]*className=\{styles\.content\}/, 'all dock actions share one rail outside the composer content slot');
assert.match(dockStyles, /\.content,[\s\S]*\[data-testid='composer'\][\s\S]*width: 100%;[\s\S]*min-width: 0;/, 'composer and permission share a stable dock width');
assert.match(workspace, /<ConversationDock[\s\S]*node: <TodoTaskBar[\s\S]*<Composer/s, 'Todo is dock-owned rather than a timeline/process row');
assert.doesNotMatch(workspace, /<Timeline[^>]*TodoTaskBar/, 'Todo is never injected into Timeline');
assert.match(todo, /data-state=\{display\.state\}/);
assert.match(todoStyles, /\.taskChip\s*\{[\s\S]*height: 32px;[\s\S]*min-height: 32px;/, 'Todo uses the shared 32px dock control height');
assert.doesNotMatch(todoStyles, /\.taskChip\[data-state='completed'\]\s*\{[^}]*min-height:/, 'completed Todo keeps the shared control height');
assert.match(dockStyles, /\.iconButton,[\s\S]*width: 32px;[\s\S]*height: 32px;/, 'jump action uses the shared 32px dock control height');
assert.match(permissionStyles, /\.resolvedGate[\s\S]*margin: 2px 0;/, 'resolved permission has no nested left indentation');
assert.match(permissionStyles, /var\(--ant-color-(success|error|warning)\)/, 'permission attention uses theme tokens');
assert.match(timelineStyles, /\.processStream[\s\S]*padding: 6px 0 2px;/, 'process content retains the flat reading edge');
assert.match(dockStyles, /@media \(max-width: 820px\)[\s\S]*calc\(100% - 28px\)/, 'responsive dock keeps the timeline 14px edge');
assert.match(workspace, /AgentActivityMonitor[\s\S]*<Tooltip title=\{rightPanelOpen/, 'Agent monitor stays in the header and cannot resize the composer');

console.log('conversation cross-surface polish smoke passed');
