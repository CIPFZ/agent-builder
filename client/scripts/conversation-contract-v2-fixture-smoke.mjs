import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { resolve } from 'node:path';

const fixture = JSON.parse(await readFile(resolve('..', 'internal', 'runtime', 'testdata', 'conversation_contract_v2.json'), 'utf8'));
assert.equal(fixture.schemaVersion, 2);
assert.equal(fixture.scope, 'full');
assert.equal(typeof fixture.cursor, 'string');
assert.equal(fixture.cursor, '9007199254740993', 'cursor remains precise beyond Number.MAX_SAFE_INTEGER');
for (const collection of ['turns','messages','assistantSteps','toolCalls','toolResults','permissions','todoPlans','agentTasks','notices']) assert.ok(Array.isArray(fixture[collection]), `${collection} is always an array`);
for (const collection of ['turns','messages','assistantSteps','toolCalls','toolResults','permissions','todoPlans','agentTasks']) for (const entity of fixture[collection]) { assert.equal(typeof entity.activitySequence, 'string'); assert.equal(typeof entity.revision, 'string'); }
assert.equal(fixture.turns[0].finalMessageId, 'message-final');
assert.equal(fixture.messages.find((message) => message.id === 'message-final').phase, 'final');
assert.equal(fixture.toolCalls[0].quiet, undefined);
assert.equal(fixture.toolCalls[0].defaultExpanded, undefined);
assert.equal(fixture.todoPlans[0].items[0].id, 'todo-1');
assert.equal(fixture.agentTasks[0].teamId, 'team-1');
console.log('conversation contract v2 fixture smoke passed');
