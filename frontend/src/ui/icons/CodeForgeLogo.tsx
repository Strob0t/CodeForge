import type { JSX } from "solid-js";

interface CodeForgeLogoProps {
  size?: number;
  class?: string;
}

export function CodeForgeLogo(props: CodeForgeLogoProps): JSX.Element {
  const size = () => props.size ?? 24;
  return (
    <svg
      viewBox="0 0 32 32"
      width={size()}
      height={size()}
      fill="none"
      class={props.class}
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
    >
      {/* Anvil body */}
      <path
        d="M6 18 H26 V24 Q26 26 24 26 H8 Q6 26 6 24 Z"
        stroke-width="1.5"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-linejoin="round"
        fill="currentColor"
        opacity="0.15"
      />
      <path
        d="M6 18 H26 V24 Q26 26 24 26 H8 Q6 26 6 24 Z"
        stroke-width="1.5"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
      {/* Anvil horn (left taper) */}
      <path
        d="M6 18 Q2 18 2 20 L6 22"
        stroke-width="1.5"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
      {/* Anvil flat top */}
      <path
        d="M8 14 H24 V18 H8 Z"
        stroke-width="1.5"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-linejoin="round"
        fill="currentColor"
        opacity="0.25"
      />
      <path
        d="M8 14 H24 V18 H8 Z"
        stroke-width="1.5"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
      {/* Anvil step (right side) */}
      <path
        d="M24 16 H28 V18 H26"
        stroke-width="1.5"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
      {/* Base/feet */}
      <path
        d="M10 26 V29 H14 V26"
        stroke-width="1.5"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
      <path
        d="M18 26 V29 H22 V26"
        stroke-width="1.5"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
      {/* Hammer (accent color) */}
      <path
        d="M13 5 H19 V9 H13 Z"
        stroke-width="1.5"
        stroke="var(--cf-accent)"
        stroke-linecap="round"
        stroke-linejoin="round"
        fill="var(--cf-accent)"
        opacity="0.3"
      />
      <path
        d="M13 5 H19 V9 H13 Z"
        stroke-width="1.5"
        stroke="var(--cf-accent)"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
      <line
        x1="16"
        y1="9"
        x2="16"
        y2="14"
        stroke-width="1.5"
        stroke="currentColor"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
      {/* Sparks (accent color) */}
      <circle cx="10" cy="10" r="1" fill="var(--cf-accent)" />
      <circle cx="22" cy="8" r="1" fill="var(--cf-accent)" />
      <circle cx="8" cy="7" r="0.7" fill="var(--cf-accent)" />
      <circle cx="24" cy="11" r="0.7" fill="var(--cf-accent)" />
    </svg>
  );
}
