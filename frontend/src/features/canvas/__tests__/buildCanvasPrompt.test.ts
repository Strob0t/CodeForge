import { describe, expect, it } from "vitest";

import { buildCanvasPrompt, modelSupportsVision } from "../buildCanvasPrompt";

// ---------------------------------------------------------------------------
// buildCanvasPrompt
// ---------------------------------------------------------------------------

describe("buildCanvasPrompt", () => {
  const sampleAscii = "+---+\n| R |\n+---+";
  const sampleJson = { elements: [{ type: "rect", x: 0, y: 0 }] };

  describe("vision model (hasVision=true)", () => {
    it("excludes ASCII art from output", () => {
      const result = buildCanvasPrompt(sampleAscii, sampleJson, "Build this UI", true);
      expect(result).not.toContain(sampleAscii);
      expect(result).not.toContain("ASCII Wireframe");
    });

    it("includes JSON structured description", () => {
      const result = buildCanvasPrompt(sampleAscii, sampleJson, "Build this UI", true);
      expect(result).toContain("[Design Canvas - Structured Description]");
      expect(result).toContain(JSON.stringify(sampleJson, null, 2));
    });

    it("includes user text", () => {
      const result = buildCanvasPrompt(sampleAscii, sampleJson, "Build this UI", true);
      expect(result).toContain("Build this UI");
    });
  });

  describe("non-vision model (hasVision=false)", () => {
    it("includes ASCII art in output", () => {
      const result = buildCanvasPrompt(sampleAscii, sampleJson, "Build this UI", false);
      expect(result).toContain(sampleAscii);
      expect(result).toContain("[Design Canvas - ASCII Wireframe]");
    });

    it("includes JSON structured description", () => {
      const result = buildCanvasPrompt(sampleAscii, sampleJson, "Build this UI", false);
      expect(result).toContain("[Structured Description]");
      expect(result).toContain(JSON.stringify(sampleJson, null, 2));
    });

    it("includes user text", () => {
      const result = buildCanvasPrompt(sampleAscii, sampleJson, "Build this UI", false);
      expect(result).toContain("Build this UI");
    });
  });

  describe("empty user text", () => {
    it("vision model: no trailing user text section", () => {
      const result = buildCanvasPrompt(sampleAscii, sampleJson, "", true);
      // Should end with JSON block, no dangling newlines/text
      expect(result).toContain(JSON.stringify(sampleJson, null, 2));
      expect(result).not.toMatch(/\n\n$/);
    });

    it("non-vision model: no trailing user text section", () => {
      const result = buildCanvasPrompt(sampleAscii, sampleJson, "", false);
      expect(result).toContain(JSON.stringify(sampleJson, null, 2));
      expect(result).not.toMatch(/\n\n$/);
    });
  });

  describe("JSON always present", () => {
    it("vision=true includes JSON", () => {
      const result = buildCanvasPrompt("", { foo: "bar" }, "", true);
      expect(result).toContain('"foo": "bar"');
    });

    it("vision=false includes JSON", () => {
      const result = buildCanvasPrompt("", { foo: "bar" }, "", false);
      expect(result).toContain('"foo": "bar"');
    });
  });

  describe("whitespace-only user text treated as empty", () => {
    it("trims whitespace-only user text", () => {
      const result = buildCanvasPrompt(sampleAscii, sampleJson, "   \n  ", true);
      expect(result).not.toMatch(/\n\n$/);
    });
  });

  describe("complex JSON objects", () => {
    it("handles nested objects", () => {
      const nested = { layout: { rows: [{ id: 1, widgets: ["a", "b"] }] } };
      const result = buildCanvasPrompt("", nested, "", true);
      expect(result).toContain('"rows"');
      expect(result).toContain('"widgets"');
    });
  });
});

// ---------------------------------------------------------------------------
// modelSupportsVision
// ---------------------------------------------------------------------------

describe("modelSupportsVision", () => {
  it("returns true for gpt-4o models", () => {
    expect(modelSupportsVision("gpt-4o")).toBe(true);
    expect(modelSupportsVision("gpt-4o-mini")).toBe(true);
    expect(modelSupportsVision("openai/gpt-4o-2024-05-13")).toBe(true);
  });

  it("returns true for gpt-4-vision models", () => {
    expect(modelSupportsVision("gpt-4-vision-preview")).toBe(true);
  });

  it("returns true for claude-3 models", () => {
    expect(modelSupportsVision("claude-3-opus-20240229")).toBe(true);
    expect(modelSupportsVision("claude-3-sonnet-20240229")).toBe(true);
    expect(modelSupportsVision("claude-3-haiku-20240307")).toBe(true);
    expect(modelSupportsVision("claude-3-5-sonnet-20241022")).toBe(true);
    expect(modelSupportsVision("anthropic/claude-3-opus")).toBe(true);
  });

  it("returns true for claude-4 models", () => {
    expect(modelSupportsVision("claude-4-opus")).toBe(true);
    expect(modelSupportsVision("claude-opus-4")).toBe(true);
  });

  it("returns true for gemini models", () => {
    expect(modelSupportsVision("gemini-pro-vision")).toBe(true);
    expect(modelSupportsVision("gemini-1.5-pro")).toBe(true);
    expect(modelSupportsVision("google/gemini-2.0-flash")).toBe(true);
  });

  it("returns false for text-only models", () => {
    expect(modelSupportsVision("gpt-3.5-turbo")).toBe(false);
    expect(modelSupportsVision("gpt-4-turbo")).toBe(false);
    expect(modelSupportsVision("claude-2.1")).toBe(false);
    expect(modelSupportsVision("mistral-7b")).toBe(false);
    expect(modelSupportsVision("llama-3-70b")).toBe(false);
  });

  it("returns false for empty string", () => {
    expect(modelSupportsVision("")).toBe(false);
  });

  it("is case-insensitive", () => {
    expect(modelSupportsVision("GPT-4O")).toBe(true);
    expect(modelSupportsVision("Claude-3-Opus")).toBe(true);
    expect(modelSupportsVision("GEMINI-PRO-VISION")).toBe(true);
  });
});
