import { beforeEach, describe, expect, it } from "vitest";

import {
  addContextFile,
  clearContextFiles,
  contextFiles,
  getContextFiles,
  removeContextFile,
} from "./contextFilesStore";

describe("contextFilesStore", () => {
  beforeEach(() => {
    clearContextFiles();
  });

  it("should export addContextFile function", () => {
    expect(typeof addContextFile).toBe("function");
  });

  it("should export removeContextFile function", () => {
    expect(typeof removeContextFile).toBe("function");
  });

  it("should export clearContextFiles function", () => {
    expect(typeof clearContextFiles).toBe("function");
  });

  it("should export getContextFiles function", () => {
    expect(typeof getContextFiles).toBe("function");
  });

  it("should export contextFiles signal", () => {
    expect(typeof contextFiles).toBe("function");
  });

  it("should start with empty context files", () => {
    expect(getContextFiles()).toEqual([]);
  });

  it("should add a file to context", () => {
    addContextFile("/src/main.ts");
    expect(getContextFiles()).toEqual(["/src/main.ts"]);
  });

  it("should add multiple files to context", () => {
    addContextFile("/src/main.ts");
    addContextFile("/src/utils.ts");
    expect(getContextFiles()).toEqual(["/src/main.ts", "/src/utils.ts"]);
  });

  it("should not add duplicate files", () => {
    addContextFile("/src/main.ts");
    addContextFile("/src/main.ts");
    expect(getContextFiles()).toEqual(["/src/main.ts"]);
  });

  it("should remove a file from context", () => {
    addContextFile("/src/main.ts");
    addContextFile("/src/utils.ts");
    removeContextFile("/src/main.ts");
    expect(getContextFiles()).toEqual(["/src/utils.ts"]);
  });

  it("should handle removing a non-existent file gracefully", () => {
    addContextFile("/src/main.ts");
    removeContextFile("/src/nonexistent.ts");
    expect(getContextFiles()).toEqual(["/src/main.ts"]);
  });

  it("should clear all context files", () => {
    addContextFile("/src/main.ts");
    addContextFile("/src/utils.ts");
    clearContextFiles();
    expect(getContextFiles()).toEqual([]);
  });

  it("should return same result from contextFiles signal and getContextFiles", () => {
    addContextFile("/src/test.ts");
    expect(contextFiles()).toEqual(getContextFiles());
  });
});
