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
const contextIndicator = await readFile(new URL('../src/features/composer/ContextUsageIndicator.tsx', import.meta.url), 'utf8');
assert.match(shell, /promptSubmitQueueRef\.current\.enqueue/, 'prompt submission remains serialized');
assert.match(shell, /optimisticConversationByClientRequestId/, 'draft optimism uses the cutover overlay');
assert.match(shell, /pruneEchoedOptimisticSubmits/, 'canonical user-message echoes settle optimistic submits');
assert.match(shell, /latestViewModel\.canonicalConversationStore\?\.sessionId === adoptedSessionId[\s\S]*?preserveCanonicalConversation\(nextViewModel, latestViewModel\)/, 'submit response cannot overwrite an earlier canonical Turn event');
assert.match(shell, /const settledSubmits[\s\S]*?pruneEchoedOptimisticSubmits\(settledSubmits, settledViewModel\.canonicalConversationStore\)/, 'a slower send response cannot resurrect an optimistic Turn already settled by the canonical stream');
assert.match(shell, /conversationTarget: \{ kind: 'draft'[\s\S]*?contextUsage: undefined/, 'new-conversation drafts cannot retain a Session context estimate');
assert.match(shell, /conversationTarget: \{ kind: 'session', sessionId: sessionID \}[\s\S]*?contextUsage: undefined/, 'Session switching clears the previous Session context estimate immediately');
assert.match(shell, /contextUsage\?\.sessionId === sessionID/, 'hydrated context usage is accepted only for the selected Session');
assert.match(contextIndicator, /上下文估算/, 'context UI describes its composite number as an estimate');
assert.doesNotMatch(contextIndicator, /手动压缩|CompressOutlined|styles\.ring/, 'composer context popover has no ineffective manual compact action or duplicate ring');
assert.doesNotMatch(shell, /retargetOutputStore|hydrateOutputStore|applyOutputEvents/, 'draft adoption cannot revive the removed output store');

console.log('conversation target submit smoke passed');
