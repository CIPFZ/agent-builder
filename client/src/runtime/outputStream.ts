import type { RuntimeOutputEvent, RuntimeOutputEventsResponse } from './outputTypes.ts';

const SESSION_OUTPUT_STREAM_EVENT = 'agent-builder:output-stream';
const POLL_INTERVAL_MS = 1200;
const BATCH_FLUSH_MS = 50;

export interface SubscribeSessionOutputBridge {
  StartSessionOutputStream?: (req: { sessionId: string; streamId?: string; after?: string }) => Promise<{ streamId: string; eventName: string }>;
  StopSessionOutputStream?: (req: { streamId: string }) => Promise<boolean>;
  SessionOutputEvents?: (sessionID: string, after: string) => Promise<RuntimeOutputEventsResponse>;
}

export interface WailsRuntimeEvents {
  Events: {
    On: (name: string, listener: (event: unknown) => void) => (() => void) | void;
  };
}

export interface SubscribeSessionOutputOptions {
  sessionId: string;
  after?: string;
  bridge?: SubscribeSessionOutputBridge | null;
  loadWailsEvents?: () => Promise<WailsRuntimeEvents | null>;
  loadEventSource?: () => Promise<{ url: string; auth?: string } | null>;
  onBatch: (events: RuntimeOutputEvent[]) => void;
  onSnapshotRequired?: () => void;
}

/**
 * subscribeSessionOutput opens a per-session push channel from the runtime
 * and buffers incoming events into 50ms batches before delivering them to
 * onBatch.
 *
 * Channel priority:
 *   1. Wails desktop bridge (StartSessionOutputStream + Events.On)
 *   2. HTTP SSE via EventSource
 *   3. Polling fallback using SessionOutputEvents every 1.2s
 *
 * The returned closer stops whichever channel is active. Callers should call
 * it on session switch, unmount, or when a snapshot re-hydration replaces
 * the store.
 */
export function subscribeSessionOutput(opts: SubscribeSessionOutputOptions): () => void {
  let cancelled = false;
  let flushTimer: ReturnType<typeof setTimeout> | undefined;
  let pending: RuntimeOutputEvent[] = [];
  let closer: (() => void) | undefined;

  const flush = () => {
    if (pending.length === 0) {
      return;
    }
    const batch = mergeAdjacentDeltas(pending);
    pending = [];
    flushTimer = undefined;
    if (!cancelled) {
      opts.onBatch(batch);
    }
  };
  const enqueue = (events: RuntimeOutputEvent[]) => {
    if (cancelled || events.length === 0) return;
    pending.push(...events);
    if (flushTimer === undefined) {
      flushTimer = setTimeout(flush, BATCH_FLUSH_MS);
    }
  };
  const handleSnapshot = () => {
    opts.onSnapshotRequired?.();
  };

  const startWails = async () => {
    const bridge = opts.bridge;
    if (!bridge?.StartSessionOutputStream) return false;
    const events = await opts.loadWailsEvents?.();
    if (!events) return false;
    let started: { streamId: string; eventName: string };
    try {
      started = await bridge.StartSessionOutputStream({ sessionId: opts.sessionId, after: opts.after });
    } catch {
      return false;
    }
    const eventName = started.eventName || SESSION_OUTPUT_STREAM_EVENT;
    const off = events.Events.On(eventName, (payload) => {
      if (cancelled) return;
      const message = payload as { data?: { events?: RuntimeOutputEvent[] }; events?: RuntimeOutputEvent[] } | undefined;
      const batch = message?.data?.events ?? message?.events ?? [];
      if (!Array.isArray(batch) || batch.length === 0) return;
      for (const ev of batch) {
        if (ev.kind === 'snapshot_required' || ev.operation === 'reset') {
          handleSnapshot();
        }
      }
      enqueue(batch);
    });
    closer = () => {
      if (typeof off === 'function') {
        try { off(); } catch { /* ignore */ }
      }
      bridge.StopSessionOutputStream?.({ streamId: started.streamId }).catch(() => undefined);
    };
    return true;
  };

  const startSSE = async () => {
    if (typeof EventSource === 'undefined' || !opts.loadEventSource) return false;
    const endpoint = await opts.loadEventSource();
    if (!endpoint?.url) return false;
    let source: EventSource;
    try {
      source = new EventSource(endpoint.url, { withCredentials: false });
    } catch {
      return false;
    }
    const handler = (event: MessageEvent) => {
      if (cancelled) return;
      try {
        const parsed = JSON.parse(event.data) as RuntimeOutputEvent;
        if (parsed.kind === 'snapshot_required' || parsed.operation === 'reset') {
          handleSnapshot();
        }
        enqueue([parsed]);
      } catch { /* ignore */ }
    };
    source.addEventListener('output-event', handler as EventListener);
    source.onerror = () => {
      // Let the caller re-hydrate; the browser will auto-reconnect afterwards.
      handleSnapshot();
    };
    closer = () => {
      source.close();
    };
    return true;
  };

  const startPolling = () => {
    if (!opts.bridge?.SessionOutputEvents) {
      closer = () => undefined;
      return true;
    }
    let cursor = opts.after ?? '';
    let stopped = false;
    const tick = async () => {
      if (stopped || cancelled) return;
      try {
        const resp = await opts.bridge!.SessionOutputEvents!(opts.sessionId, cursor);
        if (resp?.cursor) cursor = resp.cursor;
        if (resp?.events?.length) {
          for (const ev of resp.events) {
            if (ev.kind === 'snapshot_required' || ev.operation === 'reset') {
              handleSnapshot();
            }
          }
          enqueue(resp.events);
        }
      } catch { /* transient failures — the next tick retries */ }
      if (!stopped && !cancelled) {
        timer = setTimeout(tick, POLL_INTERVAL_MS);
      }
    };
    let timer: ReturnType<typeof setTimeout> | undefined = setTimeout(tick, POLL_INTERVAL_MS);
    closer = () => {
      stopped = true;
      if (timer) clearTimeout(timer);
    };
    return true;
  };

  (async () => {
    if (await startWails()) return;
    if (await startSSE()) return;
    startPolling();
  })();

  return () => {
    cancelled = true;
    if (flushTimer !== undefined) {
      clearTimeout(flushTimer);
      flushTimer = undefined;
    }
    if (closer) {
      try { closer(); } catch { /* ignore */ }
    }
  };
}

/**
 * mergeAdjacentDeltas collapses runs of text-delta events targeting the same
 * message+partType into a single accumulated delta. It preserves the order
 * of non-delta events and never merges across a non-delta boundary. This is
 * safe because the reducer applies deltas by suffix append and uses
 * contentLen as an idempotency key.
 */
export function mergeAdjacentDeltas(events: RuntimeOutputEvent[]): RuntimeOutputEvent[] {
  if (events.length < 2) return events;
  const out: RuntimeOutputEvent[] = [];
  for (const event of events) {
    if (!event.textDelta) {
      out.push(event);
      continue;
    }
    const last = out[out.length - 1];
    if (last?.textDelta && last.textDelta.messageId === event.textDelta.messageId && last.textDelta.partType === event.textDelta.partType) {
      last.textDelta = {
        ...last.textDelta,
        delta: (last.textDelta.delta ?? '') + (event.textDelta.delta ?? ''),
        contentLen: Math.max(last.textDelta.contentLen ?? 0, event.textDelta.contentLen ?? 0),
      };
      continue;
    }
    out.push(event);
  }
  return out;
}
