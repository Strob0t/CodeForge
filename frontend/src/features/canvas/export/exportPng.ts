// ---------------------------------------------------------------------------
// PNG Export — SVG to PNG via offscreen canvas
// ---------------------------------------------------------------------------

/**
 * Export an SVG element to a PNG data URL at the given scale factor.
 *
 * Steps:
 * 1. Serialize SVG to XML string via XMLSerializer
 * 2. Create a Blob from the XML string
 * 3. Create an ImageBitmap from the Blob via createImageBitmap
 * 4. Draw onto an OffscreenCanvas at the desired scale
 * 5. Convert to data URL via blob -> FileReader
 *
 * @param svgElement - The SVG DOM element to export
 * @param scale - Render scale factor (default: 2 for retina quality)
 * @returns Promise resolving to a base64 data URL starting with `data:image/png;base64,`
 */
export async function exportPng(svgElement: SVGSVGElement, scale = 2): Promise<string> {
  // 1. Serialize SVG to XML string
  const serializer = new XMLSerializer();
  const svgString = serializer.serializeToString(svgElement);

  // 2. Create a Blob from the SVG XML
  const svgBlob = new Blob([svgString], { type: "image/svg+xml;charset=utf-8" });

  // 3. Create an ImageBitmap from the Blob
  const bitmap = await createImageBitmap(svgBlob);

  // 4. Draw onto an OffscreenCanvas at the scaled resolution
  const width = bitmap.width * scale;
  const height = bitmap.height * scale;
  const canvas = new OffscreenCanvas(width, height);
  const ctx = canvas.getContext("2d");

  if (!ctx) {
    bitmap.close();
    throw new Error("Failed to get 2D context from OffscreenCanvas");
  }

  ctx.scale(scale, scale);
  ctx.drawImage(bitmap, 0, 0);
  bitmap.close();

  // 5. Convert to data URL via Blob -> FileReader
  const pngBlob = await canvas.convertToBlob({ type: "image/png" });
  return blobToDataUrl(pngBlob);
}

/** Convert a Blob to a base64 data URL string. */
function blobToDataUrl(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result === "string") {
        resolve(reader.result);
      } else {
        reject(new Error("FileReader did not return a string"));
      }
    };
    reader.onerror = () => reject(new Error("FileReader failed to read blob"));
    reader.readAsDataURL(blob);
  });
}
