import { screenToSvgCoords, type SvgPoint } from "../DesignCanvas";

// Re-export SvgPoint for convenience
export type { SvgPoint };

/** Convert a PointerEvent to SVG user-space coordinates using the SVG CTM. */
export function eventToSvg(e: PointerEvent, svgEl: SVGSVGElement | undefined): SvgPoint {
  if (!svgEl) return { x: e.clientX, y: e.clientY };

  const ctm = svgEl.getScreenCTM();
  if (!ctm) return { x: e.clientX, y: e.clientY };

  return screenToSvgCoords(e.clientX, e.clientY, ctm);
}
