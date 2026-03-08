/** MIME type used for drag-and-drop feature card transfers. */
export const FEATURE_MIME = "application/x-codeforge-feature";

/** Serializable payload attached to a dragged feature card. */
export interface DragPayload {
  featureId: string;
  sourceMilestoneId: string;
  sourceIndex: number;
}

/** Encode a DragPayload as a string for dataTransfer. */
export function encodeDragPayload(payload: DragPayload): string {
  return JSON.stringify(payload);
}

/** Decode a DragPayload from dataTransfer, returning null on parse failure. */
export function decodeDragPayload(data: string): DragPayload | null {
  try {
    return JSON.parse(data) as DragPayload;
  } catch {
    return null;
  }
}
