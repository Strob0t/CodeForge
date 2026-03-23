# Feature 06: Visual Design Canvas

## Overview

Users can visually communicate design intent to AI agents by sketching wireframes, annotating screenshots, and describing layouts. The feature adds an SVG-based design canvas to the frontend with a triple-output export pipeline (PNG, ASCII art, structured JSON) and a multimodal message pipeline that carries images alongside text through the full stack.

## Design Decision

**Option D: SVG-based editor** -- Zero new dependencies, SolidJS-native, extends existing SVG patterns from AgentFlowGraph/ArchitectureGraph/AgentNetwork.

## Triple-Output Rationale

| Model Capability | Gets |
|---|---|
| Vision (Claude, GPT-4o, Gemini) | PNG screenshot + JSON description |
| Text-only strong (GPT-4, DeepSeek) | ASCII art + JSON description |
| Basic / local (Ollama) | JSON description only |

## Architecture

### Frontend Canvas

```
frontend/src/features/canvas/
  canvasTypes.ts           -- Element types, tool interface, export types
  canvasState.ts           -- SolidJS store: elements, selection, undo/redo, viewport
  DesignCanvas.tsx         -- Main SVG viewport + pointer event dispatch
  CanvasToolbar.tsx        -- Tool selector bar
  CanvasExportPanel.tsx    -- Export preview (PNG, ASCII, JSON) with copy buttons
  CanvasModal.tsx          -- Fullscreen modal wrapper
  buildCanvasPrompt.ts     -- Prompt composition based on model vision capability
  tools/
    SelectTool.ts          -- Select, move, 8-point resize handles, Shift aspect-ratio lock
    FreehandTool.ts        -- SVG path with Catmull-Rom smoothing
    RectTool.ts            -- Rectangle creation via drag
    EllipseTool.ts         -- Ellipse/circle creation
    TextTool.ts            -- Click-to-place text, inline editing via foreignObject
    AnnotateTool.ts        -- Arrow + callout annotation
    ImageTool.ts           -- Drag-to-size file upload -> base64 -> image element
    PolygonTool.ts         -- Multi-click polygon, close on first-vertex or double-click
    NodeTool.ts            -- Drag individual vertices on polygon/freehand/annotation
  export/
    exportPng.ts           -- SVG -> XMLSerializer -> offscreen canvas -> PNG base64
    exportAscii.ts         -- Element tree -> character grid rasterization -> string
    exportJson.ts          -- Element tree -> structured JSON description
```

### Multimodal Message Pipeline

```
Frontend (PNG base64 + text)
    |
    v  POST /conversations/:id/messages  {content, images[]}
Go Core (store images JSONB, forward via NATS)
    |
    v  NATS conversation.run.start  {messages[{content, images[]}]}
Python Worker (build OpenAI content-array format)
    |
    v  LiteLLM (already supports content-array)
```

### Data Model

```typescript
interface CanvasElement {
  id: string;
  type: "rect" | "ellipse" | "freehand" | "text" | "image" | "annotation" | "polygon";
  x: number; y: number; width: number; height: number;
  rotation: number; zIndex: number;
  style: ElementStyle;
  data: RectData | EllipseData | FreehandData | TextData | ImageData | AnnotationData;
}

interface MessageImage {
  data: string;        // base64 encoded
  media_type: string;  // e.g. "image/png"
  alt_text?: string;
}
```

## API Changes

### Frontend Types

- `MessageImage` interface added to `types.ts`
- `SendMessageRequest.images?: MessageImage[]`
- `ConversationMessage.images?: MessageImage[]`
- `LLMModel.supports_vision?: boolean`

### Go Domain

- `MessageImage` struct in `internal/domain/conversation/conversation.go`
- `SendMessageRequest.Images []MessageImage`
- `Message.Images json.RawMessage` (JSONB column)
- `DiscoveredModel.SupportsVision bool` (defined in `internal/port/llm/types.go`)

### Database

- Migration `075_add_message_images.sql`: `ALTER TABLE conversation_messages ADD COLUMN images JSONB;`

### NATS Payload

- `MessageImagePayload` struct in `internal/port/messagequeue/schemas.go`
- `ConversationMessagePayload.Images []MessageImagePayload`

### Python Models

- `MessageImagePayload` in `workers/codeforge/models.py`
- `ConversationMessagePayload.images: list[MessageImagePayload]`
- `_to_msg_dict` produces OpenAI content-array when images present

## Constraints

- Max 15MB per image (server-side enforcement via `MaxImageSizeBytes` in `internal/domain/conversation/conversation.go`)
- Wireframe PNGs typically 50-200KB base64
- No new frontend dependencies (pure SVG + Canvas API)
- Keyboard shortcuts: V=select, R=rect, E=ellipse, P=pen, T=text, A=annotate, I=image, G=polygon, N=node

## Related Files

- Existing SVG patterns: `AgentFlowGraph.tsx`, `ArchitectureGraph.tsx`
- Chat input: `ChatPanel.tsx`
- Go conversation: `internal/service/conversation_agent.go`
- Python history: `workers/codeforge/history.py`
