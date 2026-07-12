import type { CanonicalProcessEntity } from './canonicalConversationSelectors.ts';
import type { CanonicalToolCall } from './canonicalConversationTypes.ts';

export interface CanonicalProcessItem { type: 'entity'; key: string; entity: Exclude<CanonicalProcessEntity, CanonicalToolCall> }
export interface CanonicalToolGroup { type: 'tool-group'; key: string; turnId: string; presentationKind: string; tools: CanonicalToolCall[] }
export type CanonicalPresentationItem = CanonicalProcessItem | CanonicalToolGroup;

export function groupCanonicalProcess(process: CanonicalProcessEntity[]): CanonicalPresentationItem[] {
  const output: CanonicalPresentationItem[] = [];
  const groups = new Map<string, CanonicalToolGroup>();
  for (const entity of process) {
    if (!isToolCall(entity)) {
      output.push({ type: 'entity', key: `${canonicalProcessEntityType(entity)}:${entity.id}`, entity });
      continue;
    }
    const key = canonicalToolGroupKey(entity);
    let group = groups.get(key);
    if (!group) {
      group = { type: 'tool-group', key, turnId: entity.turnId ?? '', presentationKind: presentationKind(entity), tools: [] };
      groups.set(key, group);
      output.push(group);
    }
    group.tools.push(entity);
  }
  return output;
}

export function canonicalToolGroupKey(tool: CanonicalToolCall) {
  const scope = tool.assistantStepId ?? tool.roundId ?? tool.messageId ?? 'unscoped';
  return `tool-group:${tool.turnId ?? ''}:${scope}:${presentationKind(tool)}`;
}

function presentationKind(tool: CanonicalToolCall) { return tool.kind || tool.source || 'tool'; }
function isToolCall(entity: CanonicalProcessEntity): entity is CanonicalToolCall { return 'name' in entity && 'source' in entity; }
function canonicalProcessEntityType(entity: Exclude<CanonicalProcessEntity, CanonicalToolCall>) {
  if ('toolCallId' in entity && 'ordinal' in entity) return 'toolResult';
  if ('toolCallId' in entity) return 'permission';
  if ('messageId' in entity && 'index' in entity) return 'assistantStep';
  if ('role' in entity) return 'message';
  if ('kind' in entity) return 'notice';
  return 'agentTask';
}
