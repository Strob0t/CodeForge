import type { JSX } from "solid-js";

interface IconProps {
  size?: number;
  class?: string;
}

/** Paperclip/attach icon (Heroicons 20x20 solid). */
export function AttachIcon(props: IconProps): JSX.Element {
  const size = () => props.size ?? 16;
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 20 20"
      fill="currentColor"
      width={size()}
      height={size()}
      class={props.class}
      aria-hidden="true"
    >
      <path
        fill-rule="evenodd"
        d="M15.621 4.379a3 3 0 0 0-4.242 0l-7 7a3 3 0 0 0 4.241 4.243l7-7a1.5 1.5 0 0 0-2.121-2.122l-7 7a.5.5 0 1 1-.707-.707l7-7a3 3 0 0 1 4.242 4.243l-7 7a5 5 0 0 1-7.071-7.071l7-7a1 1 0 0 1 1.414 1.414l-7 7a3 3 0 1 0 4.243 4.243l7-7a1.5 1.5 0 0 0 0-2.122Z"
        clip-rule="evenodd"
      />
    </svg>
  );
}

/** Pencil/canvas icon (Heroicons 20x20 solid). */
export function CanvasIcon(props: IconProps): JSX.Element {
  const size = () => props.size ?? 16;
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 20 20"
      fill="currentColor"
      width={size()}
      height={size()}
      class={props.class}
      aria-hidden="true"
    >
      <path d="M15.993 1.385a1.87 1.87 0 0 1 2.623 2.622l-4.03 4.031-2.622-2.623 4.03-4.03ZM3.74 12.104l7.217-7.216 2.623 2.622-7.217 7.217H3.74v-2.623Z" />
      <path
        fill-rule="evenodd"
        d="M0 4a2 2 0 0 1 2-2h7a1 1 0 0 1 0 2H2v14h14v-7a1 1 0 1 1 2 0v7a2 2 0 0 1-2 2H2a2 2 0 0 1-2-2V4Z"
        clip-rule="evenodd"
      />
    </svg>
  );
}
