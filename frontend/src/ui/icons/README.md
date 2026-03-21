# Icon Components (FIX-106)

## Current State

Inline SVG icons are duplicated across multiple components (ChatPanel,
WarRoom, toolbar buttons, etc.). Only `CodeForgeLogo` and `EmptyStateIcons`
have been extracted into this directory.

## TODO

Extract all shared SVG icons from feature components into reusable
components in this directory. Each icon should be a SolidJS component
accepting `class` and `size` props for consistent styling.

Priority files to extract from:
- `features/project/ChatPanel.tsx` (copy, retry, send, stop icons)
- `features/project/WarRoom.tsx` (status indicators)
- `features/canvas/CanvasToolbar.tsx` (tool icons)
- `features/settings/` (navigation icons)

Pattern to follow: see `CodeForgeLogo.tsx` for the component structure.
