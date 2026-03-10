const STORAGE_PREFIX = "codeforge:freq:";

interface FrequencyTracker {
  track(key: string): void;
  getFrequency(key: string): number;
  getAll(): Map<string, number>;
}

function loadData(storageKey: string): Record<string, number> {
  try {
    const raw = localStorage.getItem(storageKey);
    if (raw === null) {
      return {};
    }
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
      return {};
    }
    return parsed as Record<string, number>;
  } catch {
    return {};
  }
}

function saveData(storageKey: string, data: Record<string, number>): void {
  try {
    localStorage.setItem(storageKey, JSON.stringify(data));
  } catch {
    // Storage full or unavailable — silently ignore
  }
}

export function useFrequencyTracker(namespace: string): FrequencyTracker {
  const storageKey = `${STORAGE_PREFIX}${namespace}`;

  function track(key: string): void {
    const data = loadData(storageKey);
    data[key] = (data[key] ?? 0) + 1;
    saveData(storageKey, data);
  }

  function getFrequency(key: string): number {
    const data = loadData(storageKey);
    return data[key] ?? 0;
  }

  function getAll(): Map<string, number> {
    const data = loadData(storageKey);
    return new Map(Object.entries(data));
  }

  return { track, getFrequency, getAll };
}
