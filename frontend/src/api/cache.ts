// ---------------------------------------------------------------------------
// Offline Cache — in-memory GET response cache + mutation action queue
// ---------------------------------------------------------------------------

interface CacheEntry<T> {
  data: T;
  timestamp: number;
}

interface QueuedAction {
  id: string;
  path: string;
  init: RequestInit;
  resolve: (value: unknown) => void;
  reject: (reason: unknown) => void;
}

const DEFAULT_MAX_AGE_MS = 5 * 60 * 1000; // 5 minutes

// ---------------------------------------------------------------------------
// Response Cache (GET only)
// ---------------------------------------------------------------------------

const responseCache = new Map<string, CacheEntry<unknown>>();

export function getCached<T>(path: string, maxAgeMs = DEFAULT_MAX_AGE_MS): T | undefined {
  const entry = responseCache.get(path);
  if (!entry) return undefined;
  if (Date.now() - entry.timestamp > maxAgeMs) {
    responseCache.delete(path);
    return undefined;
  }
  return entry.data as T;
}

export function setCached<T>(path: string, data: T): void {
  responseCache.set(path, { data, timestamp: Date.now() });
}

export function invalidateCache(pathPrefix: string): void {
  for (const key of responseCache.keys()) {
    if (key.startsWith(pathPrefix)) {
      responseCache.delete(key);
    }
  }
}

export function clearCache(): void {
  responseCache.clear();
}

// ---------------------------------------------------------------------------
// Offline Action Queue (mutations)
// ---------------------------------------------------------------------------

const actionQueue: QueuedAction[] = [];
let isProcessing = false;

export function queueAction(path: string, init: RequestInit): Promise<unknown> {
  return new Promise((resolve, reject) => {
    const id = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    actionQueue.push({ id, path, init, resolve, reject });
  });
}

export function getQueueLength(): number {
  return actionQueue.length;
}

export async function processQueue(
  executor: (path: string, init: RequestInit) => Promise<unknown>,
): Promise<{ succeeded: number; failed: number }> {
  if (isProcessing || actionQueue.length === 0) {
    return { succeeded: 0, failed: 0 };
  }

  isProcessing = true;
  let succeeded = 0;
  let failed = 0;

  while (actionQueue.length > 0) {
    const action = actionQueue[0];
    try {
      const result = await executor(action.path, action.init);
      actionQueue.shift();
      action.resolve(result);
      succeeded++;
    } catch {
      actionQueue.shift();
      action.reject(new Error("Queued action failed after retry"));
      failed++;
    }
  }

  isProcessing = false;
  return { succeeded, failed };
}
