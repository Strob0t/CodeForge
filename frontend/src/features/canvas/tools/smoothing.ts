// ---------------------------------------------------------------------------
// Catmull-Rom spline -> SVG cubic bezier path (pure math, no DOM)
// ---------------------------------------------------------------------------

/**
 * Convert an array of 2D points into a smooth SVG path string using
 * Catmull-Rom to cubic bezier conversion.
 *
 * - 0 points -> ""
 * - 1 point  -> "M x y L x y" (a dot)
 * - 2 points -> "M x1 y1 L x2 y2" (straight line)
 * - 3+ points -> "M x1 y1 C ... C ..." (smooth curve)
 *
 * Uses standard Catmull-Rom to bezier conversion with tension factor 1/6.
 */
export function catmullRomToSvgPath(points: [number, number][]): string {
  if (points.length === 0) return "";

  const [x0, y0] = points[0];

  if (points.length === 1) {
    return `M ${x0} ${y0} L ${x0} ${y0}`;
  }

  if (points.length === 2) {
    const [x1, y1] = points[1];
    return `M ${x0} ${y0} L ${x1} ${y1}`;
  }

  // For 3+ points, convert each segment between consecutive points
  // using the Catmull-Rom to cubic bezier formula.
  // We pad the array by duplicating the first and last points so every
  // interior segment p[i]..p[i+1] has neighbors p[i-1] and p[i+2].
  const pts: [number, number][] = [points[0], ...points, points[points.length - 1]];

  let d = `M ${points[0][0]} ${points[0][1]}`;

  // Tension factor: 1/6 gives a standard Catmull-Rom interpolation
  const t = 1 / 6;

  for (let i = 1; i < pts.length - 2; i++) {
    const p0 = pts[i - 1];
    const p1 = pts[i];
    const p2 = pts[i + 1];
    const p3 = pts[i + 2];

    // Control point 1: p1 + (p2 - p0) * t
    const cp1x = round(p1[0] + (p2[0] - p0[0]) * t);
    const cp1y = round(p1[1] + (p2[1] - p0[1]) * t);

    // Control point 2: p2 - (p3 - p1) * t
    const cp2x = round(p2[0] - (p3[0] - p1[0]) * t);
    const cp2y = round(p2[1] - (p3[1] - p1[1]) * t);

    d += ` C ${cp1x} ${cp1y} ${cp2x} ${cp2y} ${round(p2[0])} ${round(p2[1])}`;
  }

  return d;
}

/** Round to 2 decimal places to keep SVG paths compact. */
function round(n: number): number {
  return Math.round(n * 100) / 100;
}
