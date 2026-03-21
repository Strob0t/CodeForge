import { describe, expect, it } from "vitest";

import type { ActionRule } from "./actionRules";
import { deriveActions } from "./actionRules";

describe("actionRules", () => {
  describe("deriveActions", () => {
    it("should return an array", () => {
      const result = deriveActions("unknown_tool", "ok");
      expect(Array.isArray(result)).toBe(true);
    });

    it("should return empty array for unrecognized tool names", () => {
      const result = deriveActions("foobar", "success");
      expect(result).toHaveLength(0);
    });

    // After file edits: suggest running tests and showing diff
    it("should suggest 'Run tests' and 'Show diff' after edit tool", () => {
      const result = deriveActions("file_edit", "ok");
      expect(result.length).toBeGreaterThanOrEqual(2);
      const labels = result.map((a: ActionRule) => a.label);
      expect(labels).toContain("Run tests");
      expect(labels).toContain("Show diff");
    });

    it("should suggest 'Run tests' after write tool", () => {
      const result = deriveActions("write_file", "ok");
      const labels = result.map((a: ActionRule) => a.label);
      expect(labels).toContain("Run tests");
    });

    it("should be case-insensitive for tool name matching", () => {
      const result = deriveActions("EDIT", "ok");
      expect(result.length).toBeGreaterThanOrEqual(2);
    });

    // After bash/exec with error: suggest fix & retry
    it("should suggest 'Fix & retry' after bash tool with error result", () => {
      const result = deriveActions("bash", "Error: command not found");
      const labels = result.map((a: ActionRule) => a.label);
      expect(labels).toContain("Fix & retry");
    });

    it("should NOT suggest 'Fix & retry' after bash tool with success result", () => {
      const result = deriveActions("bash", "success output");
      const labels = result.map((a: ActionRule) => a.label);
      expect(labels).not.toContain("Fix & retry");
    });

    it("should suggest 'Fix & retry' for exec tool with error", () => {
      const result = deriveActions("exec_command", "fatal error occurred");
      const labels = result.map((a: ActionRule) => a.label);
      expect(labels).toContain("Fix & retry");
    });

    // After search/grep/glob: suggest refine search
    it("should suggest 'Refine search' after search tool", () => {
      const result = deriveActions("code_search", "found 5 matches");
      const labels = result.map((a: ActionRule) => a.label);
      expect(labels).toContain("Refine search");
    });

    it("should suggest 'Refine search' after grep tool", () => {
      const result = deriveActions("grep_files", "match");
      const labels = result.map((a: ActionRule) => a.label);
      expect(labels).toContain("Refine search");
    });

    it("should suggest 'Refine search' after glob tool", () => {
      const result = deriveActions("glob_pattern", "files");
      const labels = result.map((a: ActionRule) => a.label);
      expect(labels).toContain("Refine search");
    });

    // After read: suggest editing
    it("should suggest 'Edit this file' after read tool", () => {
      const result = deriveActions("read_file", "file contents here");
      const labels = result.map((a: ActionRule) => a.label);
      expect(labels).toContain("Edit this file");
    });

    // Action shape validation
    it("should return ActionRule objects with correct shape", () => {
      const result = deriveActions("edit", "ok");
      for (const action of result) {
        expect(action).toHaveProperty("label");
        expect(action).toHaveProperty("action");
        expect(action).toHaveProperty("value");
        expect(typeof action.label).toBe("string");
        expect(typeof action.value).toBe("string");
        expect(["send_message", "run_tool", "navigate"]).toContain(action.action);
      }
    });

    // All current actions use "send_message"
    it("should use 'send_message' action type for all derived actions", () => {
      const tools = ["edit", "bash", "search", "read"];
      for (const tool of tools) {
        const result = deriveActions(tool, "error occurred");
        for (const action of result) {
          expect(action.action).toBe("send_message");
        }
      }
    });

    // Multiple categories can match
    it("should combine suggestions when tool name matches multiple categories", () => {
      // A tool name that includes both "read" and "search"
      const result = deriveActions("read_search_tool", "result");
      const labels = result.map((a: ActionRule) => a.label);
      expect(labels).toContain("Edit this file");
      expect(labels).toContain("Refine search");
    });

    // Empty strings
    it("should return empty array for empty tool name", () => {
      const result = deriveActions("", "");
      expect(result).toHaveLength(0);
    });
  });
});
