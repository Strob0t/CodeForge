import { createResource, createSignal, For, type JSX, onMount, Show } from "solid-js";

import { api } from "~/api/client";
import { useToast } from "~/components/Toast";
import { Card } from "~/ui/composites/Card";
import { EmptyState } from "~/ui/composites/EmptyState";
import { Modal } from "~/ui/composites/Modal";
import { BrainBookIcon } from "~/ui/icons/EmptyStateIcons";
import { PageLayout } from "~/ui/layout/PageLayout";
import { Alert } from "~/ui/primitives/Alert";
import { Badge, type BadgeVariant } from "~/ui/primitives/Badge";
import { Button, type ButtonSize, type ButtonVariant } from "~/ui/primitives/Button";
import { Input } from "~/ui/primitives/Input";
import { Skeleton } from "~/ui/primitives/Skeleton";
import { Spinner, type SpinnerSize } from "~/ui/primitives/Spinner";
import { StatusDot } from "~/ui/primitives/StatusDot";

// ---------------------------------------------------------------------------
// Color palette data — mirrors --cf-* tokens from index.css
// ---------------------------------------------------------------------------

interface ColorToken {
  variable: string;
  label: string;
}

const COLOR_GROUPS: { title: string; tokens: ColorToken[] }[] = [
  {
    title: "Surfaces",
    tokens: [
      { variable: "--cf-bg-primary", label: "bg-primary" },
      { variable: "--cf-bg-surface", label: "bg-surface" },
      { variable: "--cf-bg-surface-alt", label: "bg-surface-alt" },
      { variable: "--cf-bg-secondary", label: "bg-secondary" },
      { variable: "--cf-bg-inset", label: "bg-inset" },
    ],
  },
  {
    title: "Borders",
    tokens: [
      { variable: "--cf-border", label: "border" },
      { variable: "--cf-border-subtle", label: "border-subtle" },
      { variable: "--cf-border-input", label: "border-input" },
    ],
  },
  {
    title: "Text",
    tokens: [
      { variable: "--cf-text-primary", label: "text-primary" },
      { variable: "--cf-text-secondary", label: "text-secondary" },
      { variable: "--cf-text-tertiary", label: "text-tertiary" },
      { variable: "--cf-text-muted", label: "text-muted" },
    ],
  },
  {
    title: "Accent",
    tokens: [
      { variable: "--cf-accent", label: "accent" },
      { variable: "--cf-accent-hover", label: "accent-hover" },
      { variable: "--cf-accent-fg", label: "accent-fg" },
    ],
  },
  {
    title: "Success",
    tokens: [
      { variable: "--cf-success", label: "success" },
      { variable: "--cf-success-bg", label: "success-bg" },
      { variable: "--cf-success-fg", label: "success-fg" },
      { variable: "--cf-success-border", label: "success-border" },
    ],
  },
  {
    title: "Warning",
    tokens: [
      { variable: "--cf-warning", label: "warning" },
      { variable: "--cf-warning-bg", label: "warning-bg" },
      { variable: "--cf-warning-fg", label: "warning-fg" },
      { variable: "--cf-warning-border", label: "warning-border" },
    ],
  },
  {
    title: "Danger",
    tokens: [
      { variable: "--cf-danger", label: "danger" },
      { variable: "--cf-danger-bg", label: "danger-bg" },
      { variable: "--cf-danger-fg", label: "danger-fg" },
      { variable: "--cf-danger-border", label: "danger-border" },
    ],
  },
  {
    title: "Info",
    tokens: [
      { variable: "--cf-info", label: "info" },
      { variable: "--cf-info-bg", label: "info-bg" },
      { variable: "--cf-info-fg", label: "info-fg" },
      { variable: "--cf-info-border", label: "info-border" },
    ],
  },
  {
    title: "Interactive",
    tokens: [{ variable: "--cf-focus-ring", label: "focus-ring" }],
  },
  {
    title: "Status",
    tokens: [
      { variable: "--cf-status-running", label: "status-running" },
      { variable: "--cf-status-idle", label: "status-idle" },
      { variable: "--cf-status-waiting", label: "status-waiting" },
      { variable: "--cf-status-error", label: "status-error" },
      { variable: "--cf-status-planning", label: "status-planning" },
    ],
  },
];

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const BUTTON_VARIANTS: ButtonVariant[] = [
  "primary",
  "secondary",
  "danger",
  "ghost",
  "link",
  "icon",
  "pill",
];
const BUTTON_SIZES: ButtonSize[] = ["xs", "sm", "md", "lg"];

const BADGE_VARIANTS: BadgeVariant[] = [
  "default",
  "primary",
  "success",
  "warning",
  "danger",
  "info",
  "error",
  "neutral",
];

const SPINNER_SIZES: SpinnerSize[] = ["sm", "md", "lg"];

const STATUS_COLORS: { label: string; color: string }[] = [
  { label: "Running", color: "var(--cf-status-running)" },
  { label: "Idle", color: "var(--cf-status-idle)" },
  { label: "Waiting", color: "var(--cf-status-waiting)" },
  { label: "Error", color: "var(--cf-status-error)" },
  { label: "Planning", color: "var(--cf-status-planning)" },
];

// ---------------------------------------------------------------------------
// Section wrapper (lightweight, no Card — the page itself is the guide)
// ---------------------------------------------------------------------------

function DSSection(props: {
  id: string;
  title: string;
  description: string;
  children: JSX.Element;
}): JSX.Element {
  return (
    <section id={props.id} class="mb-12">
      <h2 class="text-xl font-semibold text-cf-text-primary mb-1">{props.title}</h2>
      <p class="text-sm text-cf-text-muted mb-4">{props.description}</p>
      {props.children}
    </section>
  );
}

// ---------------------------------------------------------------------------
// Color swatch
// ---------------------------------------------------------------------------

function Swatch(props: { variable: string; label: string }): JSX.Element {
  return (
    <div class="flex flex-col items-center gap-1">
      <div
        class="h-12 w-12 rounded-cf-md border border-cf-border shadow-cf-sm"
        style={{ "background-color": `var(${props.variable})` }}
        title={props.variable}
      />
      <span class="text-[10px] font-mono text-cf-text-muted leading-tight text-center break-all max-w-[72px]">
        {props.label}
      </span>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------

export default function DesignSystemPage(): JSX.Element {
  onMount(() => {
    document.title = "Design System - CodeForge";
  });

  // Dev-mode guard
  const [devMode] = createResource(() => api.health.check().then((h) => h.dev_mode === true));

  const { show: toast } = useToast();
  const [modalOpen, setModalOpen] = createSignal(false);

  return (
    <Show
      when={devMode()}
      fallback={
        <PageLayout title="Design System">
          <Alert variant="warning">
            The Design System page is only available in development mode (APP_ENV=development).
          </Alert>
        </PageLayout>
      }
    >
      <PageLayout
        title="Design System"
        description="Living style guide for CodeForge UI primitives, composites, and design tokens."
      >
        {/* Table of contents */}
        <nav class="mb-8 flex flex-wrap gap-2" aria-label="Design system sections">
          <For
            each={
              [
                ["palette", "Colors"],
                ["typography", "Typography"],
                ["buttons", "Buttons"],
                ["badges", "Badges"],
                ["cards", "Cards"],
                ["alerts", "Alerts"],
                ["empty", "Empty State"],
                ["skeletons", "Skeletons"],
                ["inputs", "Inputs"],
                ["status", "Status Dots"],
                ["spinners", "Spinners"],
                ["interactive", "Interactive"],
              ] as [string, string][]
            }
          >
            {(item) => (
              <a
                href={`#${item[0]}`}
                class="text-xs px-2 py-1 rounded-cf-sm border border-cf-border text-cf-text-secondary hover:text-cf-accent hover:border-cf-accent transition-colors"
              >
                {item[1]}
              </a>
            )}
          </For>
        </nav>

        {/* 1. Color Palette */}
        <DSSection
          id="palette"
          title="Color Palette"
          description="All --cf-* design tokens. Override these CSS custom properties for theming and white-labeling."
        >
          <For each={COLOR_GROUPS}>
            {(group) => (
              <div class="mb-6">
                <h3 class="text-sm font-medium text-cf-text-secondary mb-2">{group.title}</h3>
                <div class="flex flex-wrap gap-4">
                  <For each={group.tokens}>
                    {(token) => <Swatch variable={token.variable} label={token.label} />}
                  </For>
                </div>
              </div>
            )}
          </For>
        </DSSection>

        {/* 2. Typography */}
        <DSSection
          id="typography"
          title="Typography"
          description="Display headings use Outfit. Body text uses Source Sans 3. Code uses the system monospace stack."
        >
          <div class="space-y-4">
            <div>
              <h1 class="text-3xl font-bold text-cf-text-primary">Heading 1 (Outfit Bold)</h1>
              <p class="text-xs text-cf-text-muted mt-1 font-mono">
                font-display / text-3xl / font-bold
              </p>
            </div>
            <div>
              <h2 class="text-2xl font-semibold text-cf-text-primary">
                Heading 2 (Outfit Semibold)
              </h2>
              <p class="text-xs text-cf-text-muted mt-1 font-mono">
                font-display / text-2xl / font-semibold
              </p>
            </div>
            <div>
              <h3 class="text-xl font-semibold text-cf-text-primary">
                Heading 3 (Outfit Semibold)
              </h3>
              <p class="text-xs text-cf-text-muted mt-1 font-mono">
                font-display / text-xl / font-semibold
              </p>
            </div>
            <div>
              <h4 class="text-lg font-medium text-cf-text-primary">Heading 4 (Outfit Medium)</h4>
              <p class="text-xs text-cf-text-muted mt-1 font-mono">
                font-display / text-lg / font-medium
              </p>
            </div>
            <hr class="border-cf-border" />
            <div>
              <p class="text-sm text-cf-text-primary">
                Body text (Source Sans 3) - The quick brown fox jumps over the lazy dog.
              </p>
              <p class="text-xs text-cf-text-muted mt-1 font-mono">font-body / text-sm</p>
            </div>
            <div>
              <p class="text-xs text-cf-text-secondary">
                Small text - Used for descriptions, hints, and metadata.
              </p>
              <p class="text-xs text-cf-text-muted mt-1 font-mono">font-body / text-xs</p>
            </div>
            <div>
              <code class="text-sm font-mono bg-cf-bg-surface-alt px-2 py-1 rounded-cf-sm text-cf-text-primary">
                const x = monospaceText();
              </code>
              <p class="text-xs text-cf-text-muted mt-1 font-mono">
                font-mono / bg-cf-bg-surface-alt
              </p>
            </div>
          </div>
        </DSSection>

        {/* 3. Buttons */}
        <DSSection
          id="buttons"
          title="Buttons"
          description="All variant x size combinations. Variants: primary, secondary, danger, ghost, link, icon, pill."
        >
          <div class="space-y-6">
            <For each={BUTTON_VARIANTS}>
              {(variant) => (
                <div>
                  <h3 class="text-sm font-medium text-cf-text-secondary mb-2 capitalize">
                    {variant}
                  </h3>
                  <div class="flex flex-wrap items-center gap-3">
                    <For each={BUTTON_SIZES}>
                      {(size) => (
                        <Button variant={variant} size={size}>
                          {variant === "icon" ? "\u2605" : `${variant} ${size}`}
                        </Button>
                      )}
                    </For>
                    <Button variant={variant} size="md" loading>
                      {variant === "icon" ? "\u2605" : "Loading"}
                    </Button>
                    <Button variant={variant} size="md" disabled>
                      {variant === "icon" ? "\u2605" : "Disabled"}
                    </Button>
                  </div>
                </div>
              )}
            </For>
          </div>
        </DSSection>

        {/* 4. Badges */}
        <DSSection
          id="badges"
          title="Badges"
          description="Inline status indicators. All variants shown in default and pill mode."
        >
          <div class="space-y-4">
            <div>
              <h3 class="text-sm font-medium text-cf-text-secondary mb-2">Default (rounded)</h3>
              <div class="flex flex-wrap gap-2">
                <For each={BADGE_VARIANTS}>{(v) => <Badge variant={v}>{v}</Badge>}</For>
              </div>
            </div>
            <div>
              <h3 class="text-sm font-medium text-cf-text-secondary mb-2">Pill mode</h3>
              <div class="flex flex-wrap gap-2">
                <For each={BADGE_VARIANTS}>
                  {(v) => (
                    <Badge variant={v} pill>
                      {v} pill
                    </Badge>
                  )}
                </For>
              </div>
            </div>
          </div>
        </DSSection>

        {/* 5. Cards */}
        <DSSection
          id="cards"
          title="Cards"
          description="Compound component with Card, Card.Header, Card.Body, and Card.Footer."
        >
          <div class="grid gap-4 sm:grid-cols-2">
            <Card>
              <Card.Header>
                <h3 class="text-sm font-semibold text-cf-text-primary">Card with all parts</h3>
              </Card.Header>
              <Card.Body>
                <p class="text-sm text-cf-text-secondary">
                  This card demonstrates the compound Card pattern with Header, Body, and Footer
                  sections.
                </p>
              </Card.Body>
              <Card.Footer>
                <div class="flex justify-end gap-2">
                  <Button variant="ghost" size="sm">
                    Cancel
                  </Button>
                  <Button variant="primary" size="sm">
                    Save
                  </Button>
                </div>
              </Card.Footer>
            </Card>
            <Card>
              <Card.Body>
                <p class="text-sm text-cf-text-secondary">
                  A minimal card with only a Body. No Header, no Footer. Use this for simple content
                  containers.
                </p>
              </Card.Body>
            </Card>
          </div>
        </DSSection>

        {/* 6. Alerts */}
        <DSSection
          id="alerts"
          title="Alerts"
          description="Contextual feedback messages. Variants: error, warning, success, info. Supports an optional dismiss button."
        >
          <div class="space-y-3">
            <Alert variant="info">This is an informational alert with contextual details.</Alert>
            <Alert variant="success">Operation completed successfully.</Alert>
            <Alert variant="warning">Check your configuration before proceeding.</Alert>
            <Alert variant="error">An error occurred while processing the request.</Alert>
            <Alert variant="info" onDismiss={() => toast("info", "Alert dismissed")}>
              Dismissible alert -- click the X button.
            </Alert>
          </div>
        </DSSection>

        {/* 7. Empty State */}
        <DSSection
          id="empty"
          title="Empty State"
          description="Placeholder for pages or sections with no data. Supports optional illustration and action."
        >
          <div class="grid gap-4 sm:grid-cols-2">
            <Card>
              <Card.Body>
                <EmptyState
                  title="No items yet"
                  description="Create your first item to get started."
                  action={
                    <Button variant="primary" size="sm">
                      Create Item
                    </Button>
                  }
                />
              </Card.Body>
            </Card>
            <Card>
              <Card.Body>
                <EmptyState
                  title="Knowledge Base Empty"
                  description="Import or create documents to populate the knowledge base."
                  illustration={<BrainBookIcon />}
                  action={
                    <Button variant="secondary" size="sm">
                      Import Documents
                    </Button>
                  }
                />
              </Card.Body>
            </Card>
          </div>
        </DSSection>

        {/* 8. Skeletons */}
        <DSSection
          id="skeletons"
          title="Skeletons"
          description="Loading placeholders. Variants: text (multi-line), rect (block), circle (avatar)."
        >
          <div class="grid gap-6 sm:grid-cols-3">
            <div>
              <h3 class="text-sm font-medium text-cf-text-secondary mb-2">Text (3 lines)</h3>
              <Skeleton variant="text" lines={3} />
            </div>
            <div>
              <h3 class="text-sm font-medium text-cf-text-secondary mb-2">Rect</h3>
              <Skeleton variant="rect" width="100%" height="4rem" />
            </div>
            <div>
              <h3 class="text-sm font-medium text-cf-text-secondary mb-2">Circle</h3>
              <div class="flex gap-3">
                <Skeleton variant="circle" width="2.5rem" height="2.5rem" />
                <Skeleton variant="circle" width="3.5rem" height="3.5rem" />
              </div>
            </div>
          </div>
        </DSSection>

        {/* 9. Inputs */}
        <DSSection
          id="inputs"
          title="Inputs"
          description="Text inputs in default, error, and disabled states. Supports monospace mode."
        >
          <div class="grid gap-4 sm:grid-cols-2 max-w-2xl">
            <div>
              <label class="block text-xs font-medium text-cf-text-secondary mb-1">Default</label>
              <Input placeholder="Enter text..." />
            </div>
            <div>
              <label class="block text-xs font-medium text-cf-text-secondary mb-1">
                With value
              </label>
              <Input value="Hello, CodeForge" />
            </div>
            <div>
              <label class="block text-xs font-medium text-cf-danger-fg mb-1">Error state</label>
              <Input error placeholder="Invalid input" />
            </div>
            <div>
              <label class="block text-xs font-medium text-cf-text-muted mb-1">Disabled</label>
              <Input disabled value="Cannot edit" />
            </div>
            <div>
              <label class="block text-xs font-medium text-cf-text-secondary mb-1">Monospace</label>
              <Input mono placeholder="const foo = bar;" />
            </div>
          </div>
        </DSSection>

        {/* 10. Status Dots */}
        <DSSection
          id="status"
          title="Status Dots"
          description="Small colored circles indicating agent or task status. Supports pulse animation."
        >
          <div class="flex flex-wrap gap-6">
            <For each={STATUS_COLORS}>
              {(s) => (
                <div class="flex items-center gap-2">
                  <StatusDot color={s.color} label={s.label} />
                  <span class="text-sm text-cf-text-secondary">{s.label}</span>
                </div>
              )}
            </For>
          </div>
          <div class="mt-4">
            <h3 class="text-sm font-medium text-cf-text-secondary mb-2">With pulse</h3>
            <div class="flex flex-wrap gap-6">
              <For each={STATUS_COLORS}>
                {(s) => (
                  <div class="flex items-center gap-2">
                    <StatusDot color={s.color} label={`${s.label} (pulsing)`} pulse />
                    <span class="text-sm text-cf-text-secondary">{s.label}</span>
                  </div>
                )}
              </For>
            </div>
          </div>
        </DSSection>

        {/* 11. Spinners */}
        <DSSection
          id="spinners"
          title="Spinners"
          description="Loading indicators in three sizes: sm (16px), md (24px), lg (32px)."
        >
          <div class="flex items-end gap-6">
            <For each={SPINNER_SIZES}>
              {(size) => (
                <div class="flex flex-col items-center gap-2">
                  <Spinner size={size} />
                  <span class="text-xs text-cf-text-muted">{size}</span>
                </div>
              )}
            </For>
          </div>
        </DSSection>

        {/* 12. Interactive */}
        <DSSection
          id="interactive"
          title="Interactive"
          description="Trigger toast notifications and open a demo modal to test interactive components."
        >
          <div class="space-y-4">
            <div>
              <h3 class="text-sm font-medium text-cf-text-secondary mb-2">Toast notifications</h3>
              <div class="flex flex-wrap gap-2">
                <Button
                  variant="primary"
                  size="sm"
                  onClick={() => toast("success", "Operation completed successfully!")}
                >
                  Success toast
                </Button>
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => toast("info", "Here is some useful information.")}
                >
                  Info toast
                </Button>
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => toast("warning", "Proceed with caution.")}
                >
                  Warning toast
                </Button>
                <Button
                  variant="danger"
                  size="sm"
                  onClick={() => toast("error", "Something went wrong!")}
                >
                  Error toast
                </Button>
              </div>
            </div>
            <div>
              <h3 class="text-sm font-medium text-cf-text-secondary mb-2">Modal dialog</h3>
              <Button variant="secondary" size="sm" onClick={() => setModalOpen(true)}>
                Open demo modal
              </Button>
            </div>
          </div>
        </DSSection>

        {/* Demo modal */}
        <Modal open={modalOpen()} onClose={() => setModalOpen(false)} title="Demo Modal">
          <p class="text-sm text-cf-text-secondary mb-4">
            This is a demonstration of the Modal composite component. It supports a title bar, focus
            trapping, Escape key dismissal, and backdrop click dismissal.
          </p>
          <div class="flex justify-end gap-2">
            <Button variant="ghost" size="sm" onClick={() => setModalOpen(false)}>
              Cancel
            </Button>
            <Button
              variant="primary"
              size="sm"
              onClick={() => {
                setModalOpen(false);
                toast("success", "Modal confirmed!");
              }}
            >
              Confirm
            </Button>
          </div>
        </Modal>
      </PageLayout>
    </Show>
  );
}
