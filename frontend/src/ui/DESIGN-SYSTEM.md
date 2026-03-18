# CodeForge Design System

This document describes the design tokens, component library, and theming conventions used across the CodeForge frontend.

## Design Tokens

All visual properties are defined as CSS custom properties with the `--cf-` prefix. They live in `frontend/src/index.css` and are registered as Tailwind CSS v4 theme values in the `@theme` block, enabling usage like `bg-cf-accent` or `text-cf-text-primary`.

### Token Naming Convention

```
--cf-{category}-{variant}
```

| Category | Purpose | Examples |
|----------|---------|---------|
| `bg` | Surface / background colors | `--cf-bg-primary`, `--cf-bg-surface`, `--cf-bg-surface-alt`, `--cf-bg-secondary`, `--cf-bg-inset` |
| `text` | Text colors | `--cf-text-primary`, `--cf-text-secondary`, `--cf-text-tertiary`, `--cf-text-muted` |
| `border` | Border colors | `--cf-border`, `--cf-border-subtle`, `--cf-border-input` |
| `accent` | Brand accent | `--cf-accent`, `--cf-accent-hover`, `--cf-accent-fg` |
| `success` | Semantic success | `--cf-success`, `--cf-success-bg`, `--cf-success-fg`, `--cf-success-border` |
| `warning` | Semantic warning | `--cf-warning`, `--cf-warning-bg`, `--cf-warning-fg`, `--cf-warning-border` |
| `danger` | Semantic danger/error | `--cf-danger`, `--cf-danger-bg`, `--cf-danger-fg`, `--cf-danger-border` |
| `info` | Semantic info | `--cf-info`, `--cf-info-bg`, `--cf-info-fg`, `--cf-info-border` |
| `status` | Agent/task status | `--cf-status-running`, `--cf-status-idle`, `--cf-status-waiting`, `--cf-status-error`, `--cf-status-planning` |
| `focus-ring` | Keyboard focus indicator | `--cf-focus-ring` |
| `shadow` | Elevation shadows | `--cf-shadow-sm`, `--cf-shadow-md`, `--cf-shadow-lg` |
| `radius` | Border radii | `--cf-radius-sm`, `--cf-radius-md`, `--cf-radius-lg` |
| `skeleton` | Loading shimmer | `--cf-skeleton-base`, `--cf-skeleton-shine` |

### Light and Dark Modes

Light mode tokens are defined on `:root`. Dark mode overrides are inside `.dark {}`. Tailwind v4 uses the custom variant `@custom-variant dark (&:where(.dark, .dark *))` to enable class-based dark mode toggling.

## Typography

| Role | Font Family | CSS Variable |
|------|------------|-------------|
| Display (h1-h4) | Outfit | `--font-display` |
| Body text | Source Sans 3 | `--font-body` |
| Monospace / code | System monospace | `font-mono` (Tailwind default) |

Fonts are self-hosted as `.woff2` files in `/public/fonts/`.

## Components

### Primitives (`~/ui/primitives/`)

#### Button

```tsx
import { Button } from "~/ui";

<Button variant="primary" size="md">Click me</Button>
<Button variant="danger" size="sm" loading>Saving...</Button>
<Button variant="icon" size="md">X</Button>
```

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| `variant` | `"primary" \| "secondary" \| "danger" \| "ghost" \| "link" \| "icon" \| "pill"` | `"primary"` | Visual style |
| `size` | `"xs" \| "sm" \| "md" \| "lg"` | `"md"` | Size preset |
| `loading` | `boolean` | `false` | Shows spinner, disables button |
| `fullWidth` | `boolean` | `false` | Stretches to 100% width |

#### Badge

```tsx
import { Badge } from "~/ui";

<Badge variant="success">Active</Badge>
<Badge variant="danger" pill>Critical</Badge>
```

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| `variant` | `"default" \| "primary" \| "success" \| "warning" \| "danger" \| "info" \| "error" \| "neutral"` | `"default"` | Color scheme |
| `pill` | `boolean` | `false` | Fully rounded corners |

#### Input

```tsx
import { Input } from "~/ui";

<Input placeholder="Email" />
<Input error placeholder="Invalid" />
<Input mono value="code" disabled />
```

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| `error` | `boolean` | `false` | Red border + focus ring |
| `mono` | `boolean` | `false` | Monospace font |

#### Spinner

```tsx
import { Spinner } from "~/ui";

<Spinner size="sm" />
<Spinner size="md" />
<Spinner size="lg" />
```

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| `size` | `"sm" \| "md" \| "lg"` | `"md"` | 16px / 24px / 32px |

#### StatusDot

```tsx
import { StatusDot } from "~/ui";

<StatusDot color="var(--cf-status-running)" label="Running" pulse />
```

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| `color` | `string` | required | CSS color value |
| `pulse` | `boolean` | `false` | Pulse animation |
| `label` | `string` | - | Accessible label (sets `role="img"`) |

#### Skeleton

```tsx
import { Skeleton } from "~/ui";

<Skeleton variant="text" lines={4} />
<Skeleton variant="rect" width="200px" height="100px" />
<Skeleton variant="circle" width="3rem" height="3rem" />
```

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| `variant` | `"text" \| "rect" \| "circle"` | `"rect"` | Shape preset |
| `lines` | `number` | `3` | Number of text lines (text variant only) |
| `width` | `string` | varies | CSS width |
| `height` | `string` | varies | CSS height |

#### Alert

```tsx
import { Alert } from "~/ui";

<Alert variant="error">Something failed.</Alert>
<Alert variant="info" onDismiss={() => {}}>Dismissible</Alert>
```

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| `variant` | `"error" \| "warning" \| "success" \| "info"` | `"info"` | Semantic color |
| `onDismiss` | `() => void` | - | Shows dismiss button when provided |

### Composites (`~/ui/composites/`)

#### Card (compound)

```tsx
import { Card } from "~/ui";

<Card>
  <Card.Header>Title</Card.Header>
  <Card.Body>Content</Card.Body>
  <Card.Footer>Actions</Card.Footer>
</Card>
```

Sub-components: `Card.Header`, `Card.Body`, `Card.Footer`. All are optional.

#### EmptyState

```tsx
import { EmptyState } from "~/ui";
import { BrainBookIcon } from "~/ui/icons/EmptyStateIcons";

<EmptyState
  title="No data"
  description="Get started by creating something."
  illustration={<BrainBookIcon />}
  action={<Button>Create</Button>}
/>
```

#### Modal

```tsx
import { Modal } from "~/ui";

<Modal open={isOpen()} onClose={() => setOpen(false)} title="Confirm">
  <p>Are you sure?</p>
</Modal>
```

Features: focus trapping, Escape key, backdrop click dismissal, entrance animation.

### Layout (`~/ui/layout/`)

#### Section

Wraps content in a `SectionHeader` + `Card.Body`. Used for dashboard-style sections.

```tsx
import { Section } from "~/ui";

<Section title="Overview" description="Key metrics at a glance">
  <p>Content here</p>
</Section>
```

#### PageLayout

Standard page wrapper with title, description, and optional action slot.

```tsx
import { PageLayout } from "~/ui/layout/PageLayout";

<PageLayout title="Settings" description="Configure your workspace.">
  <SettingsForm />
</PageLayout>
```

## Toast Notifications

```tsx
import { useToast } from "~/components/Toast";

const { show: toast } = useToast();
toast("success", "Saved!");
toast("error", "Failed!", 5000); // custom dismiss timeout
```

Levels: `"success"`, `"error"`, `"warning"`, `"info"`. Requires `<ToastProvider>` in the component tree (already provided in `App.tsx`).

## Theme Customization

### Built-in Themes

CodeForge ships with three base themes:

1. **Solarized Light** (default light mode, defined on `:root`)
2. **VSCode Dark Modern** (default dark mode, defined on `.dark`)
3. **Nord** (custom dark theme)
4. **Solarized Dark** (custom dark theme)

### Creating a Custom Theme

Custom themes override `--cf-*` tokens via the `ThemeProvider`:

```tsx
import { useTheme } from "~/components/ThemeProvider";
import type { ThemeDefinition } from "~/ui/tokens";

const myTheme: ThemeDefinition = {
  id: "my-brand",
  name: "My Brand",
  mode: "light",         // base mode: "light" or "dark"
  tokens: {
    "--cf-accent": "#e91e63",
    "--cf-accent-hover": "#c2185b",
    "--cf-accent-fg": "#ffffff",
    // ... override any --cf-* token
  },
};

// Register and apply:
const { registerTheme, applyCustomTheme } = useTheme();
registerTheme(myTheme);
applyCustomTheme("my-brand");
```

Custom themes are persisted in `localStorage` under the key `codeforge-user-themes`.

### Theme Toggle

The `<ThemeToggle>` component cycles through light, dark, and system modes. It is included in the sidebar footer.

## Living Style Guide

Visit `/design-system` (dev mode only) to see all components rendered with every variant and size combination. This page serves as the canonical visual reference for the design system.

## Utility: `cx()`

```tsx
import { cx } from "~/utils/cx";

// Joins class names, filtering falsy values
cx("foo", false && "bar", "baz"); // => "foo baz"
```

Zero-dependency replacement for `clsx` / `classnames`.
