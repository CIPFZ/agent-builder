import assert from 'node:assert/strict';
import fs from 'node:fs';
import { initialProcessDisclosureState, reduceProcessDisclosure } from '../src/features/timeline/processDisclosurePolicy.ts';

const running = { status: 'running', hasFinalResponse: false, itemStatuses: ['running'], hasPendingPermission: false, pinned: true };
const complete = { status: 'completed', hasFinalResponse: true, itemStatuses: ['completed'], hasPendingPermission: false, pinned: true };
const waiting = { status: 'running', hasFinalResponse: false, itemStatuses: ['waiting_permission'], hasPendingPermission: true, pinned: true };
const failed = { status: 'failed', hasFinalResponse: false, itemStatuses: ['failed'], hasPendingPermission: false, pinned: true };
const terminalWithoutFinal = { ...complete, hasFinalResponse: false };

let state = initialProcessDisclosureState(running);
assert.deepEqual(state, { mode: 'auto', open: true, completionObserved: false });
state = reduceProcessDisclosure(state, { type: 'sync', signal: complete });
assert.equal(state.open, false, 'pinned completion safely auto-collapses');
state = reduceProcessDisclosure(state, { type: 'sync', signal: { ...complete, itemStatuses: ['completed', 'completed'] } });
assert.equal(state.open, false, 'late non-attention revisions retain disclosure');

let reading = initialProcessDisclosureState(running);
reading = reduceProcessDisclosure(reading, { type: 'sync', signal: { ...complete, pinned: false } });
assert.equal(reading.open, true, 'completion cannot collapse under an unpinned reader');
reading = reduceProcessDisclosure(reading, { type: 'sync', signal: complete });
assert.equal(reading.open, true, 'repinning later does not replay the consumed transition');

let manual = reduceProcessDisclosure(initialProcessDisclosureState(running), { type: 'manual', open: false });
manual = reduceProcessDisclosure(manual, { type: 'sync', signal: waiting });
assert.deepEqual({ mode: manual.mode, open: manual.open }, { mode: 'manual_closed', open: false });
manual = reduceProcessDisclosure(manual, { type: 'manual', open: true });
manual = reduceProcessDisclosure(manual, { type: 'sync', signal: complete });
assert.deepEqual({ mode: manual.mode, open: manual.open }, { mode: 'manual_open', open: true });

assert.equal(initialProcessDisclosureState(waiting).open, true);
assert.equal(initialProcessDisclosureState(failed).open, true);
assert.equal(initialProcessDisclosureState(terminalWithoutFinal).open, true, 'terminal process stays open until its final response exists');
assert.equal(initialProcessDisclosureState(complete).open, false, 'historical completed turns restore deterministic defaults');

const disclosureSource = fs.readFileSync(new URL('../src/features/timeline/ProcessDisclosure.tsx', import.meta.url), 'utf8');
const timelineStyles = fs.readFileSync(new URL('../src/features/timeline/Timeline.module.css', import.meta.url), 'utf8');
assert.match(disclosureSource, /aria-expanded=\{disclosure\.open\}/);
assert.match(disclosureSource, /hidden=\{!disclosure\.open\}/);
assert.doesNotMatch(disclosureSource, /\bCollapse\b/, 'outer disclosure must not use Ant Collapse');
assert.match(timelineStyles, /\.processStream\[hidden\]/);
assert.match(timelineStyles, /\.processTraceChevron[\s\S]*position: absolute;[\s\S]*left: -16px;/, 'chevron cannot indent the shared reading edge');

console.log('process disclosure policy smoke passed');
