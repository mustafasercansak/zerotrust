export interface Point {
  x: number;
  y: number;
}

/**
 * Calculates a smooth bezier path string connecting the given points.
 */
export function getBezierPath(pts: Point[]): string {
  if (pts.length === 0) return "";
  let d = `M ${pts[0].x} ${pts[0].y}`;
  for (let i = 0; i < pts.length - 1; i++) {
    const p0 = pts[i];
    const p1 = pts[i + 1];
    const cp1x = p0.x + (p1.x - p0.x) / 3;
    const cp1y = p0.y;
    const cp2x = p0.x + 2 * (p1.x - p0.x) / 3;
    const cp2y = p1.y;
    d += ` C ${cp1x} ${cp1y}, ${cp2x} ${cp2y}, ${p1.x} ${p1.y}`;
  }
  return d;
}

/**
 * Calculates a closed SVG path for the area under the bezier curve down to a target fillHeight.
 */
export function getBezierAreaPath(pts: Point[], fillHeight: number): string {
  const linePath = getBezierPath(pts);
  if (!linePath || pts.length === 0) return "";
  return `${linePath} L ${pts[pts.length - 1].x} ${fillHeight} L ${pts[0].x} ${fillHeight} Z`;
}
