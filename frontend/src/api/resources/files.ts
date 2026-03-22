import type { CoreClient } from "../core";
import { url } from "../factory";
import type { FileContent, FileEntry } from "../types";

export function createFilesResource(c: CoreClient) {
  return {
    list: (projectId: string, path = ".") =>
      c.get<FileEntry[]>(
        `/projects/${encodeURIComponent(projectId)}/files?path=${encodeURIComponent(path)}`,
      ),

    tree: (projectId: string, maxEntries = 10000) =>
      c.get<FileEntry[]>(
        `/projects/${encodeURIComponent(projectId)}/files/tree?max_entries=${maxEntries}`,
      ),

    read: (projectId: string, path: string) =>
      c.get<FileContent>(
        `/projects/${encodeURIComponent(projectId)}/files/content?path=${encodeURIComponent(path)}`,
      ),

    write: (projectId: string, path: string, content: string) =>
      c.put<{ status: string }>(url`/projects/${projectId}/files/content`, { path, content }),

    delete: (projectId: string, path: string) =>
      c.request<undefined>(
        `/projects/${encodeURIComponent(projectId)}/files?path=${encodeURIComponent(path)}`,
        { method: "DELETE" },
      ),

    rename: (projectId: string, oldPath: string, newPath: string) =>
      c.patch<{ status: string }>(url`/projects/${projectId}/files/rename`, {
        old_path: oldPath,
        new_path: newPath,
      }),
  };
}
