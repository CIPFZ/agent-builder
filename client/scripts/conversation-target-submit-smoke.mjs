import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createConversationSubmitQueue } from '../src/runtime/conversationSubmitQueue.ts';

// Two immediate submits must observe an atomic draft -> Session transition.
const queue = createConversationSubmitQueue();
let target = { kind: 'draft' };
const requests = [];
const submit = (prompt) => queue.enqueue(async () => {
  requests.push({ prompt, sessionId: target.kind === 'session' ? target.sessionId : undefined });
  await Promise.resolve();
  target = { kind: 'session', sessionId: 'session-1' };
});
await Promise.all([submit('hello'), submit('hi')]);
assert.deepEqual(requests, [
  { prompt: 'hello', sessionId: undefined },
  { prompt: 'hi', sessionId: 'session-1' },
], 'consecutive sends create one Session and append two Turns to it');

const shell = await readFile(new URL('../src/app/shell/WorkbenchShell.tsx', import.meta.url), 'utf8');
assert.match(shell, /promptSubmitQueueRef\.current\.enqueue/, 'prompt submission remains serialized');
assert.match(shell, /optimisticConversationByClientRequestId/, 'draft optimism uses the cutover overlay');
assert.match(shell, /pruneEchoedOptimisticSubmits/, 'canonical user-message echoes settle optimistic submits');
assert.doesNotMatch(shell, /retargetOutputStore|hydrateOutputStore|applyOutputEvents/, 'draft adoption cannot revive the removed output store');

console.log('conversation target submit smoke passed');
