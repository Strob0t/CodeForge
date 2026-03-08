/** Palette for multi-series charts (radar, bar, line). */
export const CHART_COLORS = [
  "#3B82F6", // blue
  "#EF4444", // red
  "#10B981", // green
  "#F59E0B", // amber
  "#8B5CF6", // violet
  "#EC4899", // pink
  "#06B6D4", // cyan
  "#F97316", // orange
] as const;

/** Default geometry for the SVG radar/spider chart. */
export const RADAR_DEFAULTS = {
  cx: 150,
  cy: 150,
  radius: 120,
  levels: 5,
  viewBox: "0 0 300 300",
} as const;
