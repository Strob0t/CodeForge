import { createSignal } from "solid-js";

type Breakpoint = "mobile" | "tablet" | "desktop";

interface BreakpointState {
  isMobile: () => boolean;
  isTablet: () => boolean;
  isDesktop: () => boolean;
  breakpoint: () => Breakpoint;
}

const [bp, setBp] = createSignal<Breakpoint>(detectBreakpoint());

let listenersAttached = false;

function detectBreakpoint(): Breakpoint {
  if (typeof window === "undefined") return "desktop";
  if (window.matchMedia("(max-width: 639px)").matches) return "mobile";
  if (window.matchMedia("(max-width: 1023px)").matches) return "tablet";
  return "desktop";
}

function attachListeners(): void {
  if (listenersAttached) return;
  listenersAttached = true;

  const mqMobile = window.matchMedia("(max-width: 639px)");
  const mqTablet = window.matchMedia("(min-width: 640px) and (max-width: 1023px)");

  const update = () => setBp(detectBreakpoint());

  mqMobile.addEventListener("change", update);
  mqTablet.addEventListener("change", update);
}

export function useBreakpoint(): BreakpointState {
  attachListeners();

  return {
    isMobile: () => bp() === "mobile",
    isTablet: () => bp() === "tablet",
    isDesktop: () => bp() === "desktop",
    breakpoint: bp,
  };
}
