import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const adapter = await readFile(new URL('../src/runtime/wailsWorkbenchAdapter.ts', import.meta.url), 'utf8');
const timelineMessage = await readFile(new URL('../src/features/timeline/TimelineMessage.tsx', import.meta.url), 'utf8');
const canonicalTypes = await readFile(new URL('../src/runtime/canonicalConversationTypes.ts', import.meta.url), 'utf8');

assert.match(canonicalTypes, /contentTruncated\?: boolean/, 'canonical Message declares bounded content state');
assert.match(adapter, /SessionConversationMessageContentV2/, 'full Message content uses a Wails Runtime operation');
assert.match(adapter, /response\.sessionId !== sessionID \|\| response\.messageId !== messageID/, 'content response ownership is checked again in the adapter');
assert.match(timelineMessage, /item\.contentTruncated/, 'only truncated Messages expose full-content loading');
assert.match(timelineMessage, /加载完整消息/, 'truncated Message has an explicit user action');
assert.doesNotMatch(adapter, /fetch\(|XMLHttpRequest|axios/, 'Message content loading cannot add a browser transport fallback');

console.log('message content window smoke passed');
