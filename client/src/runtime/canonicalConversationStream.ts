import type { CanonicalConversationEventBatch } from './canonicalConversationTypes.ts';

const CANONICAL_STREAM_EVENT = 'agent-builder:conversation-v2-stream';

export interface CanonicalConversationStreamBridge {
  StartSessionConversationStreamV2: (req: { sessionId: string; streamId?: string; after: string }) => Promise<{ streamId: string; eventName: string }>;
  StopSessionConversationStreamV2: (req: { streamId: string }) => Promise<boolean>;
}

export interface CanonicalWailsEvents { Events: { On: (name: string, listener: (event: unknown) => void) => (() => void) | void } }

export interface SubscribeCanonicalConversationOptions {
  sessionId: string;
  after: string;
  bridge: CanonicalConversationStreamBridge;
  loadWailsEvents: () => Promise<CanonicalWailsEvents>;
  onBatch: (batch: CanonicalConversationEventBatch) => void;
  onTransportFailure: () => void;
}

export function subscribeCanonicalConversation(opts: SubscribeCanonicalConversationOptions): () => void {
  let cancelled = false;
  let closer: (() => void) | undefined;
  const streamId = `canonical-conversation-${opts.sessionId}-${Date.now()}-${Math.random().toString(36).slice(2)}`;
  void (async () => {
    let off: (() => void) | void = undefined;
    try {
      const runtime = await opts.loadWailsEvents();
      // Register before Start so catch-up batches cannot outrun the listener.
      off = runtime.Events.On(CANONICAL_STREAM_EVENT, (payload) => {
        if (cancelled) return;
        const envelope = payload as { data?: CanonicalConversationEventBatch & { streamId?: string; lifecycle?: string }; streamId?: string; lifecycle?: string } | undefined;
        const data = envelope?.data ?? envelope;
        if (!data || data.streamId !== streamId) return;
        if (data.lifecycle === 'stream_closed') { opts.onTransportFailure(); return; }
        opts.onBatch(data as CanonicalConversationEventBatch);
      });
      const started = await opts.bridge.StartSessionConversationStreamV2({ sessionId: opts.sessionId, streamId, after: opts.after });
      if (cancelled) {
        await opts.bridge.StopSessionConversationStreamV2({ streamId: started.streamId });
        if (typeof off === 'function') off();
        return;
      }
      closer = () => {
        if (typeof off === 'function') off();
        void opts.bridge.StopSessionConversationStreamV2({ streamId: started.streamId }).catch(() => undefined);
      };
    } catch {
      if (typeof off === 'function') off();
      if (!cancelled) opts.onTransportFailure();
    }
  })();
  return () => { cancelled = true; closer?.(); };
}
