import { WsEnvelope } from "./protocol";

export type EventHandler = (message: WsEnvelope) => void;

export class MyclawdClient {
  private socket?: WebSocket;
  private readonly pending = new Map<
    string,
    { method: string; resolve: (message: WsEnvelope) => void; reject: (error: Error) => void }
  >();
  private readonly subscribers = new Set<EventHandler>();
  private sequence = 0;

  constructor(private readonly endpoint: string) {}

  connect(connectPayload: Record<string, unknown>): Promise<void> {
    return new Promise((resolve, reject) => {
      const socket = new WebSocket(this.endpoint);
      this.socket = socket;
      socket.onopen = async () => {
        try {
          await this.request("connect", connectPayload);
          resolve();
        } catch (error) {
          reject(error instanceof Error ? error : new Error("connect failed"));
        }
      };
      socket.onmessage = (event) => {
        const message = JSON.parse(String(event.data)) as WsEnvelope;
        if (message.type === "res" && message.id) {
          const pending = this.pending.get(message.id);
          if (pending) {
            this.pending.delete(message.id);
            if (message.ok) {
              pending.resolve(message);
            } else {
              pending.reject(new Error(`${pending.method}: ${message.error?.message ?? "request failed"}`));
            }
            return;
          }
        }
        if (message.type === "event") {
          for (const handler of this.subscribers) {
            handler(message);
          }
        }
      };
      socket.onerror = () => {
        reject(new Error(`websocket error connecting to ${this.endpoint}`));
      };
      socket.onclose = () => {
        for (const entry of this.pending.values()) {
          entry.reject(new Error("socket closed"));
        }
        this.pending.clear();
      };
    });
  }

  disconnect(): void {
    this.socket?.close();
    this.socket = undefined;
  }

  subscribe(handler: EventHandler): () => void {
    this.subscribers.add(handler);
    return () => {
      this.subscribers.delete(handler);
    };
  }

  async request(method: string, payload: Record<string, unknown> = {}): Promise<WsEnvelope> {
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) {
      throw new Error("socket not connected");
    }
    const id = `req-${++this.sequence}`;
    const message: WsEnvelope = {
      type: "req",
      id,
      method,
      payload,
    };
    const response = new Promise<WsEnvelope>((resolve, reject) => {
      this.pending.set(id, { method, resolve, reject });
    });
    this.socket.send(JSON.stringify(message));
    return response;
  }
}
