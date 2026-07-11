import type { RuntimeOutputEvent } from './outputTypes.ts';

const SESSION_OUTPUT_STREAM_EVENT = 'agent-builder:output-stream';
const BATCH_FLUSH_MS = 50;

export interface SubscribeSessionOutputBridge {
  StartSessionOutputStream: (req: { sessionId: string; streamId?: string; after?: string }) => Promise<{ streamId: string; eventName: string }>;
  StopSessionOutputStream: (req: { streamId: string }) => Promise<boolean>;
}

export interface WailsRuntimeEvents {
  Events: {
    On: (name: string, listener: (event: unknown) => void) => (() => void) | void;
  };
}

export interface SubscribeSessionOutputOptions {
  sessionId: string;
  after?: string;
  bridge: SubscribeSessionOutputBridge;
  loadWailsEvents: () => Promise<WailsRuntimeEvents>;
  onBatch: (events: RuntimeOutputEvent[]) => void;
  onSnapshotRequired?: () => void;
}

/**
 * Opens the single supported output transport: a Wails desktop event stream.
 * Events are buffered into 50ms batches before delivery to reduce React work.
 */
export function subscribeSessionOutput(opts: SubscribeSessionOutputOptions): () => void {
  let cancelled = false;
  let flushTimer: ReturnType<typeof setTimeout> | undefined;
  let pending: RuntimeOutputEvent[] = [];
  let closer: (() => void) | undefined;
  const streamId = `session-output-${opts.sessionId}-${Date.now()}-${Math.random().toString(36).slice(2)}`;

  const flush = () => {
    if (pending.length === 0) return;
    const batch = mergeAdjacentDeltas(pending);
    pending = [];
    flushTimer = undefined;
    if (!cancelled) opts.onBatch(batch);
  };
  const enqueue = (events: RuntimeOutputEvent[]) => {
    if (cancelled || events.length === 0) return;
    pending.push(...events);
    if (flushTimer === undefined) flushTimer = setTimeout(flush, BATCH_FLUSH_MS);
  };

  void (async () => {
    let off: (() => void) | void = undefined;
    try {
      const events = await opts.loadWailsEvents();
      off = events.Events.On(SESSION_OUTPUT_STREAM_EVENT, (payload) => {
        if (cancelled) return;
        const message = payload as { data?: { streamId?: string; events?: RuntimeOutputEvent[] }; streamId?: string; events?: RuntimeOutputEvent[] } | undefined;
        const data = message?.data ?? message;
        if (data?.streamId !== streamId) return;
        const batch = data?.events ?? [];
        if (!Array.isArray(batch) || batch.length === 0) return;
        if (batch.some((event) => event.kind === 'snapshot_required' || event.operation === 'reset')) opts.onSnapshotRequired?.();
        enqueue(batch);
      });
      const started = await opts.bridge.StartSessionOutputStream({ sessionId: opts.sessionId, streamId, after: opts.after });
      if (cancelled) {
        await opts.bridge.StopSessionOutputStream({ streamId: started.streamId });
        if (typeof off === 'function') off();
        return;
      }
      closer = () => {
        if (typeof off === 'function') {
          try { off(); } catch { /* Wails listener was already removed. */ }
        }
        void opts.bridge.StopSessionOutputStream({ streamId: started.streamId }).catch(() => undefined);
      };
    } catch {
      if (typeof off === 'function') off();
      opts.onSnapshotRequired?.();
    }
  })();

  return () => {
    cancelled = true;
    if (flushTimer !== undefined) clearTimeout(flushTimer);
    closer?.();
  };
}

/** Preserve fragment order so reducer-level contentLen guards remain valid. */
export function mergeAdjacentDeltas(events: RuntimeOutputEvent[]): RuntimeOutputEvent[] {
  return events;
}
