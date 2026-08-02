import { applyCanonicalConversationBatch, applyCanonicalConversationDeltas, compareDecimal, hydrateCanonicalConversationStore, type CanonicalConversationStore } from './canonicalConversationStore.ts';
import type { CanonicalConversationEventBatch, CanonicalConversationSnapshot, CanonicalConversationSnapshotRequest } from './canonicalConversationTypes.ts';

const INITIAL_TURN_WINDOW = 30;
const MAX_CACHED_SESSIONS = 2;

export interface CanonicalConversationCoordinatorDeps {
  fetchSnapshot: (sessionId: string, request?: CanonicalConversationSnapshotRequest) => Promise<CanonicalConversationSnapshot>;
  subscribe: (sessionId: string, after: string, handlers: { onBatch: (batch: CanonicalConversationEventBatch) => void; onTransportFailure: () => void }) => Promise<() => void> | (() => void);
  onStore: (store: CanonicalConversationStore) => void;
  retryDelayMs?: (attempt: number) => number;
  cursorGraceMs?: number;
}

export interface CanonicalConversationCoordinator {
  activate: (sessionId: string) => void;
  ensureCursor: (sessionId: string, cursor: string) => void;
  loadEarlier: (sessionId: string) => Promise<boolean>;
  loadAround: (sessionId: string, turnId: string) => Promise<boolean>;
  evict: (sessionId: string) => void;
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
  let cursorCheckTimer: ReturnType<typeof setTimeout> | undefined;
  let expectedCursor = '0';
  const loadingEarlier = new Set<string>();

  const remember = (sessionId: string, store: CanonicalConversationStore) => {
    cache.delete(sessionId);
    cache.set(sessionId, store);
    while (cache.size > MAX_CACHED_SESSIONS) {
      const oldest = cache.keys().next().value as string | undefined;
      if (!oldest) break;
      cache.delete(oldest);
    }
  };

  const stopStream = () => { const current = close; close = undefined; current?.(); };
  const clearRetry = () => { if (retryTimer) clearTimeout(retryTimer); retryTimer = undefined; };
  const clearCursorCheck = () => { if (cursorCheckTimer) clearTimeout(cursorCheckTimer); cursorCheckTimer = undefined; expectedCursor = '0'; };
  const current = (sessionId: string, gen: number) => activeSessionId === sessionId && generation === gen;

  const connect = async (sessionId: string, gen: number, store: CanonicalConversationStore) => {
    if (!current(sessionId, gen)) return;
    const cleanup = await deps.subscribe(sessionId, store.cursor, {
      onBatch: (batch) => {
        if (!current(sessionId, gen) || recovering) return;
        const base = cache.get(sessionId);
        if (!base) return;
        const durable = applyCanonicalConversationBatch(base, batch);
        const next = applyCanonicalConversationDeltas(durable, batch.deltas);
        if (next.recovery) {
          void recover(sessionId, gen);
          return;
        }
        remember(sessionId, next);
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
      const snapshot = await deps.fetchSnapshot(sessionId, { scope: 'window', limit: INITIAL_TURN_WINDOW });
      if (!current(sessionId, recoveryGeneration)) return;
      const store = hydrateCanonicalConversationStore(snapshot, cache.get(sessionId));
      if (store.recovery) throw new Error(`canonical snapshot recovery failed: ${store.recovery.reason}`);
      remember(sessionId, store);
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
      clearCursorCheck();
      stopStream();
      // Historical pagination deliberately grows only the active Session.
      // Once the user leaves it, do not retain that expanded history in the
      // LRU: a later activation can cheaply reconstruct the newest window and
      // page upward again. Normal one-window Sessions remain hot-cached.
      if (activeSessionId && activeSessionId !== sessionId) {
        const previous = cache.get(activeSessionId);
        if (previous && Object.keys(previous.turnsById).length > INITIAL_TURN_WINDOW) {
          cache.delete(activeSessionId);
        } else if (previous && Object.keys(previous.streamingByMessageId).length > 0) {
          // Live deltas belong only to the actively subscribed Session. Drop
          // them when leaving; a later activation rehydrates durable content
          // before reconnecting and cannot retain stale token buffers.
          cache.set(activeSessionId, { ...previous, streamingByMessageId: {} });
        }
      }
      activeSessionId = sessionId;
      recovering = false;
      retryAttempt = 0;
      const gen = ++generation;
      if (!sessionId) return;
      const cached = cache.get(sessionId);
      if (cached) {
        remember(sessionId, cached);
        deps.onStore(cached);
        void connect(sessionId, gen, cached).catch(() => { if (current(sessionId, gen)) void recover(sessionId, gen); });
      } else {
        void recover(sessionId, gen);
      }
    },
    ensureCursor(sessionId, cursor) {
      if (sessionId !== activeSessionId || !/^\d+$/.test(cursor) || cursor === '0') return;
      if (compareDecimal(cursor, expectedCursor) > 0) expectedCursor = cursor;
      if (cursorCheckTimer) clearTimeout(cursorCheckTimer);
      const gen = generation;
      cursorCheckTimer = setTimeout(() => {
        cursorCheckTimer = undefined;
        if (!current(sessionId, gen)) return;
        const expected = expectedCursor;
        expectedCursor = '0';
        const store = cache.get(sessionId);
        // The general Runtime stream is an independent Wails invalidation
        // channel. If it observed a durable event that the canonical stream
        // has not delivered, recover from the authoritative SQLite snapshot
        // instead of leaving the active Session stale until it is re-opened.
        if (!store || compareDecimal(store.cursor, expected) < 0) void recover(sessionId, gen);
      }, deps.cursorGraceMs ?? 500);
    },
    async loadEarlier(sessionId) {
      if (sessionId !== activeSessionId || loadingEarlier.has(sessionId)) return false;
      const base = cache.get(sessionId);
      const before = base?.window?.turnIds?.[0];
      if (!base || !base.window?.hasMoreBefore || !before) return false;
      loadingEarlier.add(sessionId);
      try {
        const snapshot = await deps.fetchSnapshot(sessionId, { scope: 'window', limit: INITIAL_TURN_WINDOW, before });
        if (sessionId !== activeSessionId) return false;
        const currentStore = cache.get(sessionId);
        if (!currentStore) return false;
        const next = hydrateCanonicalConversationStore(snapshot, currentStore);
        if (next.recovery) return false;
        remember(sessionId, next);
        deps.onStore(next);
        return true;
      } finally {
        loadingEarlier.delete(sessionId);
      }
    },
    async loadAround(sessionId, turnId) {
      if (sessionId !== activeSessionId || !turnId) return false;
      const snapshot = await deps.fetchSnapshot(sessionId, { scope: 'window', limit: INITIAL_TURN_WINDOW, around: turnId });
      if (sessionId !== activeSessionId) return false;
      const base = cache.get(sessionId);
      if (!base) return false;
      const next = hydrateCanonicalConversationStore(snapshot, base);
      if (next.recovery) return false;
      remember(sessionId, next);
      deps.onStore(next);
      return true;
    },
    evict(sessionId) { loadingEarlier.delete(sessionId); cache.delete(sessionId); },
    stop() { activeSessionId = ''; recovering = false; retryAttempt = 0; generation += 1; clearRetry(); clearCursorCheck(); stopStream(); },
    cached(sessionId) { return cache.get(sessionId); },
  };
}
