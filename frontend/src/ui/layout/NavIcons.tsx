import type { JSX } from "solid-js";

// ---------------------------------------------------------------------------
// Inline SVG nav icons (20x20, stroke-based, no icon library)
// ---------------------------------------------------------------------------

const svgBase = {
  width: "20",
  height: "20",
  viewBox: "0 0 20 20",
  fill: "none",
  stroke: "currentColor",
  "stroke-width": "1.5",
  "stroke-linecap": "round" as const,
  "stroke-linejoin": "round" as const,
  "aria-hidden": "true" as const,
} as const;

export function DashboardIcon(): JSX.Element {
  return (
    <svg {...svgBase}>
      <rect x="3" y="3" width="6" height="6" rx="1" />
      <rect x="11" y="3" width="6" height="6" rx="1" />
      <rect x="3" y="11" width="6" height="6" rx="1" />
      <rect x="11" y="11" width="6" height="6" rx="1" />
    </svg>
  );
}

export function CostsIcon(): JSX.Element {
  return (
    <svg {...svgBase}>
      <circle cx="10" cy="10" r="7" />
      <path d="M10 6v8M8 8.5c0-.8.9-1.5 2-1.5s2 .7 2 1.5-.9 1.5-2 1.5-2 .7-2 1.5.9 1.5 2 1.5" />
    </svg>
  );
}

export function ModelsIcon(): JSX.Element {
  return (
    <svg {...svgBase}>
      <path d="M10 3l6 3.5v7L10 17l-6-3.5v-7L10 3z" />
      <path d="M10 10l6-3.5M10 10v7M10 10L4 6.5" />
    </svg>
  );
}

export function ActivityIcon(): JSX.Element {
  return (
    <svg {...svgBase}>
      <polyline points="3,10 6,10 8,5 10,15 12,8 14,12 17,12" />
    </svg>
  );
}

export function KnowledgeBaseIcon(): JSX.Element {
  return (
    <svg {...svgBase}>
      <path d="M4 4h12c0 0 0 2-6 2S4 4 4 4z" />
      <path d="M4 4v10c0 0 0 2 6 2s6-2 6-2V4" />
      <path d="M4 9c0 0 0 2 6 2s6-2 6-2" />
    </svg>
  );
}

export function McpIcon(): JSX.Element {
  return (
    <svg {...svgBase}>
      <path d="M8 4v3a2 2 0 01-2 2H4M12 4v3a2 2 0 002 2h2" />
      <path d="M8 16v-3a2 2 0 00-2-2H4M12 16v-3a2 2 0 012-2h2" />
      <circle cx="10" cy="10" r="2" />
    </svg>
  );
}

export function PromptsIcon(): JSX.Element {
  return (
    <svg {...svgBase}>
      <rect x="3" y="4" width="14" height="12" rx="2" />
      <path d="M7 9l2 2-2 2M11 13h3" />
    </svg>
  );
}

export function SettingsIcon(): JSX.Element {
  return (
    <svg {...svgBase}>
      <circle cx="10" cy="10" r="3" />
      <path d="M10 3v2M10 15v2M17 10h-2M5 10H3M14.95 5.05l-1.41 1.41M6.46 13.54l-1.41 1.41M14.95 14.95l-1.41-1.41M6.46 6.46L5.05 5.05" />
    </svg>
  );
}

export function A2AIcon(): JSX.Element {
  return (
    <svg {...svgBase}>
      <circle cx="6" cy="6" r="2.5" />
      <circle cx="14" cy="6" r="2.5" />
      <circle cx="10" cy="15" r="2.5" />
      <path d="M8 7l2 6M12 7l-2 6M7.5 7.5l5-1" />
    </svg>
  );
}

export function BenchmarksIcon(): JSX.Element {
  return (
    <svg {...svgBase}>
      <rect x="3" y="11" width="3" height="5" rx="0.5" />
      <rect x="8.5" y="7" width="3" height="9" rx="0.5" />
      <rect x="14" y="4" width="3" height="12" rx="0.5" />
    </svg>
  );
}

export function QuarantineIcon(): JSX.Element {
  return (
    <svg {...svgBase}>
      <path d="M10 3L4 7v6c0 3.3 2.7 4.5 6 7 3.3-2.5 6-3.7 6-7V7l-6-4z" />
      <path d="M10 8v4M10 14h.01" />
    </svg>
  );
}

export function MicroagentsIcon(): JSX.Element {
  return (
    <svg {...svgBase}>
      <circle cx="10" cy="7" r="3" />
      <path d="M5 17c0-2.8 2.2-5 5-5s5 2.2 5 5" />
      <path d="M14 5l2-2M16 5l-2-2M6 5L4 3M4 5l2-2" />
    </svg>
  );
}

export function RoutingIcon(): JSX.Element {
  return (
    <svg {...svgBase}>
      <circle cx="5" cy="10" r="2" />
      <circle cx="15" cy="5" r="2" />
      <circle cx="15" cy="15" r="2" />
      <path d="M7 10h2c2 0 3-2 4-5M7 10h2c2 0 3 2 4 5" />
    </svg>
  );
}

export function CollapseIcon(): JSX.Element {
  return (
    <svg {...svgBase}>
      <path d="M11 4l-5 6 5 6M15 4l-5 6 5 6" />
    </svg>
  );
}

export function ExpandIcon(): JSX.Element {
  return (
    <svg {...svgBase}>
      <path d="M9 4l5 6-5 6M5 4l5 6-5 6" />
    </svg>
  );
}
