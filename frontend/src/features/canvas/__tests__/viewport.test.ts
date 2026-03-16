import { describe, expect, it } from "vitest";

import { screenToSvgCoords } from "../DesignCanvas";

// ---------------------------------------------------------------------------
// screenToSvgCoords — pure coordinate transform tests
//
// The function applies the inverse of the SVG screen CTM to convert
// client (screen) coordinates to SVG user-space coordinates.
//
// DOMMatrix: [a, b, c, d, e, f] maps to:
//   | a  c  e |
//   | b  d  f |
//   | 0  0  1 |
//
// For a typical SVG viewBox transform:
//   a = scaleX, d = scaleY, e = translateX, f = translateY
//   (b = 0, c = 0 for non-rotated/non-skewed transforms)
//
// Inverse: x_svg = (clientX - e) / a,  y_svg = (clientY - f) / d
// ---------------------------------------------------------------------------

/** Build a DOMMatrix-like object for testing (jsdom lacks DOMMatrix). */
function makeCTM(a: number, d: number, e: number, f: number): DOMMatrix {
  return { a, b: 0, c: 0, d, e, f } as DOMMatrix;
}

describe("screenToSvgCoords", () => {
  it("identity transform (zoom=1, pan=0,0) leaves coordinates unchanged", () => {
    const ctm = makeCTM(1, 1, 0, 0);
    const result = screenToSvgCoords(150, 250, ctm);

    expect(result.x).toBeCloseTo(150);
    expect(result.y).toBeCloseTo(250);
  });

  it("zoomed in (zoom=2) halves the coordinates", () => {
    // scale=2 means SVG pixels are 2x bigger on screen
    const ctm = makeCTM(2, 2, 0, 0);
    const result = screenToSvgCoords(200, 400, ctm);

    expect(result.x).toBeCloseTo(100);
    expect(result.y).toBeCloseTo(200);
  });

  it("panned (translateX=100) offsets x coordinate", () => {
    const ctm = makeCTM(1, 1, 100, 0);
    const result = screenToSvgCoords(250, 300, ctm);

    expect(result.x).toBeCloseTo(150);
    expect(result.y).toBeCloseTo(300);
  });

  it("panned (translateY=50) offsets y coordinate", () => {
    const ctm = makeCTM(1, 1, 0, 50);
    const result = screenToSvgCoords(200, 350, ctm);

    expect(result.x).toBeCloseTo(200);
    expect(result.y).toBeCloseTo(300);
  });

  it("combined zoom + pan transforms correctly", () => {
    // zoom=2, panX=100 (translate=100), panY=60 (translate=60)
    // SVG x = (clientX - translateX) / scale = (300 - 100) / 2 = 100
    // SVG y = (clientY - translateY) / scale = (260 - 60) / 2 = 100
    const ctm = makeCTM(2, 2, 100, 60);
    const result = screenToSvgCoords(300, 260, ctm);

    expect(result.x).toBeCloseTo(100);
    expect(result.y).toBeCloseTo(100);
  });

  it("zoomed out (zoom=0.5) doubles the coordinates", () => {
    const ctm = makeCTM(0.5, 0.5, 0, 0);
    const result = screenToSvgCoords(100, 100, ctm);

    expect(result.x).toBeCloseTo(200);
    expect(result.y).toBeCloseTo(200);
  });

  it("negative pan offsets correctly", () => {
    const ctm = makeCTM(1, 1, -50, -75);
    const result = screenToSvgCoords(100, 100, ctm);

    // x = (100 - (-50)) / 1 = 150
    // y = (100 - (-75)) / 1 = 175
    expect(result.x).toBeCloseTo(150);
    expect(result.y).toBeCloseTo(175);
  });

  it("handles zero coordinates", () => {
    const ctm = makeCTM(1, 1, 0, 0);
    const result = screenToSvgCoords(0, 0, ctm);

    expect(result.x).toBeCloseTo(0);
    expect(result.y).toBeCloseTo(0);
  });

  it("handles fractional zoom values precisely", () => {
    const ctm = makeCTM(1.5, 1.5, 30, 45);
    const result = screenToSvgCoords(180, 195, ctm);

    // x = (180 - 30) / 1.5 = 100
    // y = (195 - 45) / 1.5 = 100
    expect(result.x).toBeCloseTo(100);
    expect(result.y).toBeCloseTo(100);
  });
});
