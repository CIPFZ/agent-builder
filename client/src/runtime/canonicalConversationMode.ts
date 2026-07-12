import type { CanonicalConversationDiagnostics } from './canonicalConversationTypes.ts';

export function resolveCanonicalConversationEnabled(current: boolean | undefined, diagnostics?: Pick<CanonicalConversationDiagnostics, 'mode'>) {
  return diagnostics ? diagnostics.mode === 'canonical_v2' : current;
}
