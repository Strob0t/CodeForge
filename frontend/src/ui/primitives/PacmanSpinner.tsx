import { For, type JSX, splitProps } from "solid-js";

export type PacmanSpinnerSize = "sm" | "md" | "lg";

export interface PacmanSpinnerProps {
  size?: PacmanSpinnerSize;
  class?: string;
}

const sizePx: Record<PacmanSpinnerSize, number> = {
  sm: 32,
  md: 48,
  lg: 64,
};

const DOT_COUNT = 12;
const ORBIT_RADIUS = 35;

function dotProps(index: number): { cx: number; cy: number; opacity: number; scale: number } {
  const angle = index * 30;
  const rad = (angle * Math.PI) / 180;
  const cx = 50 + ORBIT_RADIUS * Math.sin(rad);
  const cy = 50 - ORBIT_RADIUS * Math.cos(rad);

  // Eat zone: 330-360 deg — fade out
  if (angle >= 330) {
    return { cx, cy, opacity: (360 - angle) / 30, scale: 1 };
  }
  // Spawn zone: 0-90 deg — scale up
  if (angle <= 90) {
    return { cx, cy, opacity: 1, scale: angle / 90 };
  }
  // Full zone: 90-330 deg
  return { cx, cy, opacity: 1, scale: 1 };
}

export function PacmanSpinner(props: PacmanSpinnerProps): JSX.Element {
  const [local] = splitProps(props, ["size", "class"]);
  const size = (): PacmanSpinnerSize => local.size ?? "md";
  const px = (): number => sizePx[size()];

  return (
    <span
      role="status"
      aria-label="Loading"
      class={"inline-block" + (local.class ? " " + local.class : "")}
    >
      <span class="sr-only">Loading</span>
      <svg width={px()} height={px()} viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg">
        {/* Pacman at 12 o'clock */}
        <g transform="translate(50, 15)">
          <circle r="12" fill="var(--cf-warning)" />
          <path
            d="M 0 0 L 12 -6 A 12 12 0 0 0 12 6 Z"
            fill="var(--cf-bg-primary)"
            style={{
              animation: "cf-pacman-chomp 0.25s linear infinite",
              "transform-origin": "0 0",
            }}
          />
        </g>

        {/* Orbiting dots */}
        <g
          style={{ animation: "cf-dot-orbit 3s linear infinite", "transform-origin": "50px 50px" }}
        >
          <For each={Array.from({ length: DOT_COUNT }, (_, i) => i)}>
            {(i) => {
              const d = dotProps(i);
              return (
                <circle
                  cx={d.cx}
                  cy={d.cy}
                  r={4}
                  fill="var(--cf-accent)"
                  opacity={d.opacity}
                  transform={`scale(${d.scale})`}
                  transform-origin={`${d.cx} ${d.cy}`}
                />
              );
            }}
          </For>
        </g>
      </svg>
    </span>
  );
}
