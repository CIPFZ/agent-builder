import assert from 'node:assert/strict';
import { performance } from 'node:perf_hooks';
import { hydrateCanonicalConversationStore } from '../src/runtime/canonicalConversationStore.ts';
import { selectCanonicalConversationTurnViewModels } from '../src/runtime/canonicalConversationView.ts';

const sizes = [100, 1_000, 10_000];
const results = [];

for (const size of sizes) {
  global.gc?.();
  const heapBefore = process.memoryUsage().heapUsed;
  const snapshot = fixture(size);
  const hydrateStarted = performance.now();
  const store = hydrateCanonicalConversationStore(snapshot);
  const hydrateMs = performance.now() - hydrateStarted;
  const projectStarted = performance.now();
  const turns = selectCanonicalConversationTurnViewModels(store);
  const projectMs = performance.now() - projectStarted;
  const heapDeltaMB = (process.memoryUsage().heapUsed - heapBefore) / (1024 * 1024);
  assert.equal(turns.length, size);
  results.push({ turns: size, hydrateMs: round(hydrateMs), projectMs: round(projectMs), heapDeltaMB: round(heapDeltaMB) });
}

const largest = results.at(-1);
assert.ok(largest.hydrateMs < 5_000, `10k Turn hydration regression: ${largest.hydrateMs}ms`);
assert.ok(largest.projectMs < 5_000, `10k Turn projection regression: ${largest.projectMs}ms`);
assert.ok(largest.heapDeltaMB < 256, `10k Turn normalized-store regression: ${largest.heapDeltaMB}MB`);
console.log(JSON.stringify({ benchmark: 'canonical-conversation', node: process.version, results }, null, 2));

function fixture(size) {
  const turns = [];
  const messages = [];
  const toolCalls = [];
  const toolResults = [];
  for (let index = 0; index < size; index += 1) {
    const turnId = `turn-${index}`;
    const userId = `user-${index}`;
    const finalId = `final-${index}`;
    const toolId = `tool-${index}`;
    const resultId = `result-${index}`;
    const base = index * 5 + 1;
    turns.push(meta(turnId, turnId, base, { status: 'completed', userMessageId: userId, finalMessageId: finalId }));
    messages.push(meta(userId, turnId, base + 1, { role: 'user', phase: 'intermediate', status: 'completed', content: `request ${index} ${'x'.repeat(128)}` }));
    messages.push(meta(finalId, turnId, base + 4, { role: 'assistant', phase: 'final', status: 'completed', content: `response ${index} ${'y'.repeat(256)}` }));
    toolCalls.push(meta(toolId, turnId, base + 2, { name: 'read', source: 'builtin', kind: 'read', status: 'completed', resultIds: [resultId] }));
    toolResults.push(meta(resultId, turnId, base + 3, { toolCallId: toolId, ordinal: 0, status: 'completed', contentPreview: 'ok' }));
  }
  return { schemaVersion: 2, sessionId: 'benchmark-session', cursor: String(size * 5), scope: 'window', window: { turnIds: turns.map((turn) => turn.id), hasMoreBefore: false }, turns, messages, assistantSteps: [], toolCalls, toolResults, permissions: [], todoPlans: [], agentTasks: [], notices: [] };
}

function meta(id, turnId, sequence, fields) { return { id, sessionId: 'benchmark-session', turnId, activitySequence: String(sequence), revision: String(sequence), createdAt: sequence, updatedAt: sequence, ...fields }; }
function round(value) { return Math.round(value * 100) / 100; }
