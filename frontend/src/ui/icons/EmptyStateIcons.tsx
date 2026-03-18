import type { JSX } from "solid-js";

/** Stylized server with plug/cable for MCP empty state. */
export function ServerPlugIcon(): JSX.Element {
  return (
    <svg
      viewBox="0 0 120 120"
      width="120"
      height="120"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
    >
      {/* Server body */}
      <rect x="25" y="28" width="50" height="64" rx="4" stroke="currentColor" stroke-width="2" />
      {/* Rack dividers */}
      <line x1="25" y1="48" x2="75" y2="48" stroke="currentColor" stroke-width="1.5" />
      <line x1="25" y1="68" x2="75" y2="68" stroke="currentColor" stroke-width="1.5" />
      {/* Drive indicators */}
      <circle cx="35" cy="38" r="2.5" fill="var(--cf-accent)" />
      <circle cx="35" cy="58" r="2.5" fill="var(--cf-accent)" />
      <circle cx="35" cy="78" r="2.5" fill="currentColor" opacity="0.3" />
      {/* Drive slots */}
      <rect
        x="42"
        y="35"
        width="20"
        height="6"
        rx="1"
        stroke="currentColor"
        stroke-width="1"
        opacity="0.5"
      />
      <rect
        x="42"
        y="55"
        width="20"
        height="6"
        rx="1"
        stroke="currentColor"
        stroke-width="1"
        opacity="0.5"
      />
      <rect
        x="42"
        y="75"
        width="20"
        height="6"
        rx="1"
        stroke="currentColor"
        stroke-width="1"
        opacity="0.5"
      />
      {/* Plug cable */}
      <path
        d="M75 55 C85 55 85 45 95 45"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
      />
      {/* Plug head */}
      <rect x="93" y="39" width="12" height="12" rx="2" stroke="currentColor" stroke-width="2" />
      <line
        x1="98"
        y1="42"
        x2="98"
        y2="48"
        stroke="var(--cf-accent)"
        stroke-width="2"
        stroke-linecap="round"
      />
      <line
        x1="102"
        y1="42"
        x2="102"
        y2="48"
        stroke="var(--cf-accent)"
        stroke-width="2"
        stroke-linecap="round"
      />
    </svg>
  );
}

/** Brain / open book icon for Knowledge empty state. */
export function BrainBookIcon(): JSX.Element {
  return (
    <svg
      viewBox="0 0 120 120"
      width="120"
      height="120"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
    >
      {/* Book spine */}
      <path d="M60 30 L60 95" stroke="currentColor" stroke-width="2" />
      {/* Left page */}
      <path
        d="M60 30 C60 30 45 28 25 32 L25 90 C45 86 60 90 60 90"
        stroke="currentColor"
        stroke-width="2"
        fill="var(--cf-accent)"
        fill-opacity="0.08"
      />
      {/* Right page */}
      <path
        d="M60 30 C60 30 75 28 95 32 L95 90 C75 86 60 90 60 90"
        stroke="currentColor"
        stroke-width="2"
        fill="var(--cf-accent)"
        fill-opacity="0.08"
      />
      {/* Text lines left */}
      <line x1="35" y1="45" x2="52" y2="43" stroke="currentColor" stroke-width="1" opacity="0.4" />
      <line x1="35" y1="53" x2="52" y2="51" stroke="currentColor" stroke-width="1" opacity="0.4" />
      <line x1="35" y1="61" x2="52" y2="59" stroke="currentColor" stroke-width="1" opacity="0.4" />
      <line
        x1="35"
        y1="69"
        x2="48"
        y2="67.5"
        stroke="currentColor"
        stroke-width="1"
        opacity="0.4"
      />
      {/* Text lines right */}
      <line x1="68" y1="43" x2="85" y2="45" stroke="currentColor" stroke-width="1" opacity="0.4" />
      <line x1="68" y1="51" x2="85" y2="53" stroke="currentColor" stroke-width="1" opacity="0.4" />
      <line x1="68" y1="59" x2="85" y2="61" stroke="currentColor" stroke-width="1" opacity="0.4" />
      {/* Brain sparkle */}
      <circle cx="78" cy="72" r="3" fill="var(--cf-accent)" opacity="0.6" />
      <path
        d="M78 66 L78 69 M78 75 L78 78 M72 72 L75 72 M81 72 L84 72"
        stroke="var(--cf-accent)"
        stroke-width="1.5"
        stroke-linecap="round"
      />
    </svg>
  );
}

/** Bar chart with trophy/star for Benchmarks empty state. */
export function ChartTrophyIcon(): JSX.Element {
  return (
    <svg
      viewBox="0 0 120 120"
      width="120"
      height="120"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
    >
      {/* Chart bars */}
      <rect
        x="22"
        y="70"
        width="14"
        height="25"
        rx="2"
        stroke="currentColor"
        stroke-width="2"
        fill="var(--cf-accent)"
        fill-opacity="0.1"
      />
      <rect
        x="42"
        y="50"
        width="14"
        height="45"
        rx="2"
        stroke="currentColor"
        stroke-width="2"
        fill="var(--cf-accent)"
        fill-opacity="0.15"
      />
      <rect
        x="62"
        y="35"
        width="14"
        height="60"
        rx="2"
        stroke="currentColor"
        stroke-width="2"
        fill="var(--cf-accent)"
        fill-opacity="0.2"
      />
      <rect
        x="82"
        y="55"
        width="14"
        height="40"
        rx="2"
        stroke="currentColor"
        stroke-width="2"
        fill="var(--cf-accent)"
        fill-opacity="0.12"
      />
      {/* Baseline */}
      <line
        x1="18"
        y1="95"
        x2="100"
        y2="95"
        stroke="currentColor"
        stroke-width="1.5"
        opacity="0.5"
      />
      {/* Trophy / star */}
      <path
        d="M69 22 L71.5 27 L77 27.8 L73 31.7 L74 37.2 L69 34.5 L64 37.2 L65 31.7 L61 27.8 L66.5 27 Z"
        fill="var(--cf-accent)"
        stroke="var(--cf-accent)"
        stroke-width="1"
      />
    </svg>
  );
}

/** Document with pen/pencil for Prompts empty state. */
export function DocumentPenIcon(): JSX.Element {
  return (
    <svg
      viewBox="0 0 120 120"
      width="120"
      height="120"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
    >
      {/* Document */}
      <path
        d="M30 20 L75 20 L90 35 L90 100 L30 100 Z"
        stroke="currentColor"
        stroke-width="2"
        fill="var(--cf-accent)"
        fill-opacity="0.06"
      />
      {/* Folded corner */}
      <path
        d="M75 20 L75 35 L90 35"
        stroke="currentColor"
        stroke-width="2"
        fill="var(--cf-accent)"
        fill-opacity="0.1"
      />
      {/* Text lines */}
      <line
        x1="42"
        y1="50"
        x2="78"
        y2="50"
        stroke="currentColor"
        stroke-width="1.5"
        opacity="0.4"
      />
      <line
        x1="42"
        y1="60"
        x2="72"
        y2="60"
        stroke="currentColor"
        stroke-width="1.5"
        opacity="0.4"
      />
      <line
        x1="42"
        y1="70"
        x2="75"
        y2="70"
        stroke="currentColor"
        stroke-width="1.5"
        opacity="0.4"
      />
      <line
        x1="42"
        y1="80"
        x2="60"
        y2="80"
        stroke="currentColor"
        stroke-width="1.5"
        opacity="0.4"
      />
      {/* Pen */}
      <path
        d="M85 68 L100 53 L104 57 L89 72 Z"
        stroke="var(--cf-accent)"
        stroke-width="2"
        fill="var(--cf-accent)"
        fill-opacity="0.2"
      />
      <path
        d="M85 68 L83 75 L89 72"
        fill="var(--cf-accent)"
        stroke="var(--cf-accent)"
        stroke-width="1"
      />
      <line
        x1="100"
        y1="53"
        x2="104"
        y2="57"
        stroke="var(--cf-accent)"
        stroke-width="2"
        stroke-linecap="round"
      />
    </svg>
  );
}

/** Heartbeat / pulse line for Activity empty state. */
export function TimelinePulseIcon(): JSX.Element {
  return (
    <svg
      viewBox="0 0 120 120"
      width="120"
      height="120"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
    >
      {/* Timeline base */}
      <line x1="10" y1="60" x2="110" y2="60" stroke="currentColor" stroke-width="1" opacity="0.2" />
      {/* Pulse line */}
      <polyline
        points="10,60 30,60 38,60 42,40 48,80 54,35 60,75 66,45 70,60 80,60 110,60"
        stroke="var(--cf-accent)"
        stroke-width="2.5"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
      {/* Timeline dots */}
      <circle cx="20" cy="85" r="3" stroke="currentColor" stroke-width="1.5" fill="none" />
      <circle
        cx="45"
        cy="85"
        r="3"
        stroke="currentColor"
        stroke-width="1.5"
        fill="var(--cf-accent)"
        fill-opacity="0.3"
      />
      <circle
        cx="70"
        cy="85"
        r="3"
        stroke="currentColor"
        stroke-width="1.5"
        fill="var(--cf-accent)"
        fill-opacity="0.6"
      />
      <circle
        cx="95"
        cy="85"
        r="3"
        stroke="currentColor"
        stroke-width="1.5"
        fill="currentColor"
        opacity="0.2"
      />
      {/* Connecting line between dots */}
      <line x1="23" y1="85" x2="42" y2="85" stroke="currentColor" stroke-width="1" opacity="0.3" />
      <line x1="48" y1="85" x2="67" y2="85" stroke="currentColor" stroke-width="1" opacity="0.3" />
      <line x1="73" y1="85" x2="92" y2="85" stroke="currentColor" stroke-width="1" opacity="0.3" />
    </svg>
  );
}

/** Stacked coins / wallet for Cost Dashboard empty state. */
export function CoinsWalletIcon(): JSX.Element {
  return (
    <svg
      viewBox="0 0 120 120"
      width="120"
      height="120"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
    >
      {/* Wallet body */}
      <rect
        x="20"
        y="35"
        width="65"
        height="50"
        rx="4"
        stroke="currentColor"
        stroke-width="2"
        fill="var(--cf-accent)"
        fill-opacity="0.06"
      />
      {/* Wallet flap */}
      <path
        d="M20 45 L20 35 C20 33 22 31 24 31 L81 31 C83 31 85 33 85 35 L85 45"
        stroke="currentColor"
        stroke-width="2"
      />
      {/* Card slot */}
      <rect x="68" y="52" width="20" height="16" rx="3" stroke="currentColor" stroke-width="1.5" />
      <circle cx="78" cy="60" r="4" fill="var(--cf-accent)" opacity="0.4" />
      {/* Coins stack */}
      <ellipse
        cx="100"
        cy="82"
        rx="12"
        ry="5"
        stroke="currentColor"
        stroke-width="1.5"
        fill="var(--cf-accent)"
        fill-opacity="0.15"
      />
      <ellipse
        cx="100"
        cy="76"
        rx="12"
        ry="5"
        stroke="currentColor"
        stroke-width="1.5"
        fill="var(--cf-accent)"
        fill-opacity="0.2"
      />
      <ellipse
        cx="100"
        cy="70"
        rx="12"
        ry="5"
        stroke="currentColor"
        stroke-width="1.5"
        fill="var(--cf-accent)"
        fill-opacity="0.25"
      />
      <line x1="88" y1="70" x2="88" y2="82" stroke="currentColor" stroke-width="1.5" />
      <line x1="112" y1="70" x2="112" y2="82" stroke="currentColor" stroke-width="1.5" />
      {/* Dollar sign on top coin */}
      <text
        x="100"
        y="73"
        text-anchor="middle"
        font-size="8"
        font-weight="bold"
        fill="var(--cf-accent)"
      >
        $
      </text>
    </svg>
  );
}
