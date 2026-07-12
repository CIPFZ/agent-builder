import { applyCanonicalConversationBatch, hydrateCanonicalConversationStore, type CanonicalConversationStore } from './canonicalConversationStore.ts';
import type { CanonicalConversationEventBatch, CanonicalConversationSnapshot } from './canonicalConversationTypes.ts';

export interface CanonicalConversationCoordinatorDeps {
  fetchSnapshot: (sessionId: string) => Promise<CanonicalConversationSnapshot>;
  subscribe: (sessionId: string, after: string, handlers: { onBatch: (batch: CanonicalConversationEventBatch) => void; onTransportFailure: () => void }) => Promise<() => void> | (() => void);
  onStore: (store: CanonicalConversationStore) => void;
  retryDelayMs?: (attempt: number) => number;
}

export interface CanonicalConversationCoordinator {
  activate: (sessionId: string) => void;
  stop: () => void;
  cached: (sessionId: string) => CanonicalConversationStore | undefined;
}

export function createCanonicalConversationCoordinator(deps: CanonicalConversationCoordinatorDeps): CanonicalConversationCoordinator {
  const cache = new Map<string, CanonicalConversationStore>();
  let generation = 0;
  let activeSessionId = '';
  let close: (() => void) | undefined;
  let recovering = false;
  let retryTimer: ReturnType<typeof setTimeout> | undefined;
  let retryAttempt = 0;

  const stopStream = () => { const current = close; close = undefined; current?.(); };
  const clearRetry = () => { if (retryTimer) clearTimeout(retryTimer); retryTimer = undefined; };
  const current = (sessionId: string, gen: number) => activeSessionId === sessionId && generation === gen;

  const connect = async (sessionId: string, gen: number, store: CanonicalConversationStore) => {
    if (!current(sessionId, gen)) return;
    const cleanup = await deps.subscribe(sessionId, store.cursor, {
      onBatch: (batch) => {
        if (!current(sessionId, gen) || recovering) return;
        const base = cache.get(sessionId);
        if (!base) return;
        const next = applyCanonicalConversationBatch(base, batch);
        if (next.recovery) {
          void recover(sessionId, gen);
          return;
        }
        cache.set(sessionId, next);
        deps.onStore(next);
      },
      onTransportFailure: () => { if (current(sessionId, gen)) void recover(sessionId, gen); },
    });
    if (!current(sessionId, gen)) cleanup(); else close = cleanup;
  };

  const recover = async (sessionId: string, gen: number) => {
    if (!current(sessionId, gen) || recovering) return;
    recovering = true;
    stopStream();
    const recoveryGeneration = ++generation;
    try {
      const snapshot = await deps.fetchSnapshot(sessionId);
      if (!current(sessionId, recoveryGeneration)) return;
      const store = hydrateCanonicalConversationStore(snapshot, cache.get(sessionId));
      if (store.recovery) throw new Error(`canonical snapshot recovery failed: ${store.recovery.reason}`);
      cache.set(sessionId, store);
      deps.onStore(store);
      retryAttempt = 0;
      recovering = false;
      await connect(sessionId, recoveryGeneration, store);
    } catch {
      if (current(sessionId, recoveryGeneration)) {
        const delay = deps.retryDelayMs?.(retryAttempt) ?? Math.min(500 * (2 ** retryAttempt), 8_000);
        retryAttempt += 1;
        retryTimer = setTimeout(() => { retryTimer = undefined; void recover(sessionId, recoveryGeneration); }, delay);
      }
    } finally {
      if (current(sessionId, recoveryGeneration)) recovering = false;
    }
  };

  return {
    activate(sessionId) {
      clearRetry();
      stopStream();
      activeSessionId = sessionId;
      recovering = false;
      retryAttempt = 0;
      const gen = ++generation;
      if (!sessionId) return;
      const cached = cache.get(sessionId);
      if (cached) {
        deps.onStore(cached);
        void connect(sessionId, gen, cached);
      } else {
        void recover(sessionId, gen);
      }
    },
    stop() { activeSessionId = ''; recovering = false; retryAttempt = 0; generation += 1; clearRetry(); stopStream(); },
    cached(sessionId) { return cache.get(sessionId); },
  };
}
