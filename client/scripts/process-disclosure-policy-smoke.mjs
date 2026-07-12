import assert from 'node:assert/strict';
import fs from 'node:fs';
import { initialProcessDisclosureState, reduceProcessDisclosure } from '../src/features/timeline/processDisclosurePolicy.ts';

const running = { status: 'running', hasFinalResponse: false, itemStatuses: ['running'], hasPendingPermission: false };
const complete = { status: 'completed', hasFinalResponse: true, itemStatuses: ['completed'], hasPendingPermission: false };
const waiting = { status: 'running', hasFinalResponse: false, itemStatuses: ['waiting_permission'], hasPendingPermission: true };
const failed = { status: 'failed', hasFinalResponse: false, itemStatuses: ['failed'], hasPendingPermission: false };
const terminalWithoutFinal = { ...complete, hasFinalResponse: false };
const done = { ...complete, status: 'done' };

let state = initialProcessDisclosureState(running);
assert.deepEqual(state, { mode: 'auto', open: true, completionObserved: false });
state = reduceProcessDisclosure(state, { type: 'sync', signal: complete });
assert.equal(state.open, false, 'successful completion safely auto-collapses');
state = reduceProcessDisclosure(state, { type: 'sync', signal: { ...complete, itemStatuses: ['completed', 'completed'] } });
assert.equal(state.open, false, 'late non-attention revisions retain disclosure');

let manual = reduceProcessDisclosure(initialProcessDisclosureState(running), { type: 'manual', open: false });
manual = reduceProcessDisclosure(manual, { type: 'sync', signal: waiting });
assert.deepEqual({ mode: manual.mode, open: manual.open }, { mode: 'manual_closed', open: false });
manual = reduceProcessDisclosure(manual, { type: 'manual', open: true });
manual = reduceProcessDisclosure(manual, { type: 'sync', signal: complete });
assert.deepEqual({ mode: manual.mode, open: manual.open, completionObserved: manual.completionObserved }, { mode: 'auto', open: false, completionObserved: true }, 'completion collapses once after active-phase manual viewing');
manual = reduceProcessDisclosure(manual, { type: 'manual', open: true });
manual = reduceProcessDisclosure(manual, { type: 'sync', signal: { ...complete, itemStatuses: ['completed', 'completed'] } });
assert.deepEqual({ mode: manual.mode, open: manual.open }, { mode: 'manual_open', open: true }, 'manual choice after completion remains authoritative');

assert.equal(initialProcessDisclosureState(waiting).open, true);
assert.equal(initialProcessDisclosureState(failed).open, true);
assert.equal(initialProcessDisclosureState(terminalWithoutFinal).open, true, 'terminal process stays open until its final response exists');
assert.equal(initialProcessDisclosureState(complete).open, false, 'historical completed turns restore deterministic defaults');
assert.equal(initialProcessDisclosureState(done).open, false, 'runtime success aliases also restore collapsed defaults');

const disclosureSource = fs.readFileSync(new URL('../src/features/timeline/ProcessDisclosure.tsx', import.meta.url), 'utf8');
const timelineStyles = fs.readFileSync(new URL('../src/features/timeline/Timeline.module.css', import.meta.url), 'utf8');
assert.match(disclosureSource, /aria-expanded=\{disclosure\.open\}/);
assert.match(disclosureSource, /\{disclosure\.open && \(/, 'collapsed process details are unmounted');
assert.doesNotMatch(disclosureSource, /hidden=\{!disclosure\.open\}/);
assert.match(disclosureSource, /<ProcessLabel \{\.\.\.props\} \/><RightOutlined className=\{styles\.processTraceChevron\}/, 'chevron follows the process label');
assert.doesNotMatch(disclosureSource, /\bpinned\b/, 'disclosure must not depend on scroll position');
assert.doesNotMatch(disclosureSource, /\bCollapse\b/, 'outer disclosure must not use Ant Collapse');
assert.match(timelineStyles, /\.processTraceHeader[\s\S]*display: inline-flex;[\s\S]*align-items: center;[\s\S]*gap: 4px;/, 'header aligns its label and chevron');
assert.doesNotMatch(timelineStyles, /\.processTraceChevron\s*\{[^}]*position:\s*absolute;/, 'chevron stays in normal flex flow');
assert.match(timelineStyles, /\.processTraceChevron\s*\{[^}]*flex: 0 0 12px;[^}]*font-size: 10px;/, 'chevron is optically smaller than the process label');
assert.match(timelineStyles, /\.processTraceChevron svg\s*\{[^}]*display: block;/, 'svg chevron does not inherit text baseline alignment');

console.log('process disclosure policy smoke passed');
