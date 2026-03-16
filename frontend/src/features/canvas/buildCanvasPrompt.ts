/**
 * Builds a prompt string from canvas exports, adapting output based on
 * whether the target model supports vision (image) inputs.
 *
 * Vision models receive: structured JSON description + user text
 *   (the PNG image is sent separately as a MessageImage attachment)
 * Non-vision models receive: ASCII wireframe + JSON description + user text
 */
export function buildCanvasPrompt(
  ascii: string,
  json: object,
  userText: string,
  hasVision: boolean,
): string {
  const jsonStr = JSON.stringify(json, null, 2);
  const trimmedUserText = userText.trim();

  if (hasVision) {
    // Vision models get the image separately; only include JSON description + user text
    const parts = [`[Design Canvas - Structured Description]\n${jsonStr}`];
    if (trimmedUserText) {
      parts.push(trimmedUserText);
    }
    return parts.join("\n\n");
  }

  // Non-vision models get ASCII art + JSON + user text
  const parts = [
    `[Design Canvas - ASCII Wireframe]\n\`\`\`\n${ascii}\n\`\`\``,
    `[Structured Description]\n${jsonStr}`,
  ];
  if (trimmedUserText) {
    parts.push(trimmedUserText);
  }
  return parts.join("\n\n");
}

/** Vision-capable model name patterns (case-insensitive). */
const VISION_PATTERNS: RegExp[] = [
  /gpt-4o/i,
  /gpt-4-vision/i,
  /claude-[3-9]/i,
  /claude-\w+-[3-9]/i, // claude-opus-4, claude-sonnet-4, etc.
  /gemini/i,
];

/**
 * Heuristic to determine if a model supports vision based on its name.
 * Used as a fallback when the model's `supports_vision` property is not
 * available from the API.
 */
export function modelSupportsVision(modelName: string): boolean {
  if (!modelName) return false;
  return VISION_PATTERNS.some((pattern) => pattern.test(modelName));
}
