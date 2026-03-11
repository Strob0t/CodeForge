import { createSignal } from "solid-js";

/**
 * Lightweight store for files added to chat context via the file explorer.
 * Shared between FilePanel ("Add to Context") and ChatPanel (display + send).
 */
const [contextFiles, setContextFiles] = createSignal<string[]>([]);

export function addContextFile(path: string): void {
  setContextFiles((prev) => (prev.includes(path) ? prev : [...prev, path]));
}

export function removeContextFile(path: string): void {
  setContextFiles((prev) => prev.filter((p) => p !== path));
}

export function clearContextFiles(): void {
  setContextFiles([]);
}

export function getContextFiles(): string[] {
  return contextFiles();
}

export { contextFiles };
