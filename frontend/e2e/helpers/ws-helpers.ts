/**
 * WebSocket test utilities for E2E tests.
 * Provides a simple raw WebSocket client for verifying server-side events.
 */

const WS_BASE = "ws://localhost:8080/ws";

export interface WSMessage {
  type: string;
  payload: Record<string, unknown>;
}

/**
 * Create a WebSocket connection with JWT auth.
 * Returns helpers to collect messages, wait for specific events, and close.
 */
export function createTestWS(token: string): TestWSClient {
  return new TestWSClient(token);
}

export class TestWSClient {
  private ws: WebSocket | null = null;
  private messages: WSMessage[] = [];
  private listeners: Array<(msg: WSMessage) => void> = [];
  private openPromise: Promise<void> | null = null;
  private token: string;

  constructor(token: string) {
    this.token = token;
  }

  /** Connect to the WebSocket server. */
  async connect(): Promise<void> {
    if (this.openPromise) return this.openPromise;

    this.openPromise = new Promise<void>((resolve, reject) => {
      const url = `${WS_BASE}?token=${encodeURIComponent(this.token)}`;
      this.ws = new WebSocket(url);

      this.ws.onopen = () => resolve();
      this.ws.onerror = (ev) => reject(new Error(`WebSocket error: ${String(ev)}`));

      this.ws.onmessage = (ev: MessageEvent) => {
        const msg = JSON.parse(String(ev.data)) as WSMessage;
        this.messages.push(msg);
        for (const listener of this.listeners) {
          listener(msg);
        }
      };
    });

    return this.openPromise;
  }

  /** Connect without auth token (for negative tests). */
  async connectWithoutAuth(): Promise<void> {
    this.openPromise = new Promise<void>((resolve, reject) => {
      this.ws = new WebSocket(WS_BASE);
      this.ws.onopen = () => resolve();
      this.ws.onerror = (ev) => reject(new Error(`WebSocket error: ${String(ev)}`));
      this.ws.onclose = (ev) => {
        if (!ev.wasClean) reject(new Error(`WebSocket closed: code=${ev.code}`));
      };
    });
    return this.openPromise;
  }

  /** Wait for a message of a specific type with timeout. */
  async waitForMessage(type: string, timeoutMs = 10_000): Promise<WSMessage> {
    // Check already received messages
    const existing = this.messages.find((m) => m.type === type);
    if (existing) return existing;

    return new Promise<WSMessage>((resolve, reject) => {
      const timeout = setTimeout(() => {
        cleanup();
        reject(new Error(`Timeout waiting for WS message type="${type}" after ${timeoutMs}ms`));
      }, timeoutMs);

      const listener = (msg: WSMessage) => {
        if (msg.type === type) {
          cleanup();
          resolve(msg);
        }
      };

      const cleanup = () => {
        clearTimeout(timeout);
        const idx = this.listeners.indexOf(listener);
        if (idx >= 0) this.listeners.splice(idx, 1);
      };

      this.listeners.push(listener);
    });
  }

  /** Collect messages for a duration, then return them. */
  async collectMessages(durationMs: number): Promise<WSMessage[]> {
    const start = this.messages.length;
    await new Promise((resolve) => setTimeout(resolve, durationMs));
    return this.messages.slice(start);
  }

  /** Get all collected messages. */
  getMessages(): WSMessage[] {
    return [...this.messages];
  }

  /** Get messages of a specific type. */
  getMessagesByType(type: string): WSMessage[] {
    return this.messages.filter((m) => m.type === type);
  }

  /** Check if connected. */
  isConnected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN;
  }

  /** Close the connection. */
  close(): void {
    this.ws?.close();
    this.ws = null;
    this.openPromise = null;
  }

  /** Clear collected messages. */
  clearMessages(): void {
    this.messages = [];
  }
}
