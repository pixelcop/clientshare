export interface ImageGeometry {
  width: number;
  height: number;
  viewportWidth: number;
  viewportHeight: number;
  fitScale: number;
  minScale: number;
  maxScale: number;
}

export function imageGeometry(
  width: number,
  height: number,
  viewportWidth: number,
  viewportHeight: number,
  quarterTurns: number,
): ImageGeometry {
  const sideways = quarterTurns % 2 !== 0;
  const rotatedWidth = sideways ? height : width;
  const rotatedHeight = sideways ? width : height;
  const fit = rotatedWidth && rotatedHeight
    ? Math.min(viewportWidth / rotatedWidth, viewportHeight / rotatedHeight)
    : 1;
  const fitScale = Math.min(1, fit);
  return {
    width: rotatedWidth,
    height: rotatedHeight,
    viewportWidth,
    viewportHeight,
    fitScale,
    minScale: Math.min(0.1, fitScale),
    // Even a tiny image can be enlarged beyond the viewport before resetting.
    maxScale: Math.max(8, fit * 1.5),
  };
}

export function stepImageZoom(current: number, direction: 1 | -1, geometry: ImageGeometry) {
  const levels = [geometry.minScale, geometry.fitScale, 0.1, 0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 3, 4, 6, 8];
  for (let scale = 12; scale < geometry.maxScale; scale *= 1.5) {levels.push(scale);}
  levels.push(geometry.maxScale);
  const ordered = [...new Set(levels)]
    .filter(scale => scale >= geometry.minScale && scale <= geometry.maxScale)
    .sort((a, b) => a - b);
  return direction === 1
    ? ordered.find(scale => scale > current + 0.00001) ?? geometry.maxScale
    : ordered.reverse().find(scale => scale < current - 0.00001) ?? geometry.minScale;
}

export function imageClickZoom(current: number, geometry: ImageGeometry) {
  if (geometry.fitScale < 1) {
    return current >= 1 ? geometry.fitScale : 1;
  }
  const overflows = geometry.width * current > geometry.viewportWidth + 1 ||
    geometry.height * current > geometry.viewportHeight + 1;
  return overflows || current >= geometry.maxScale
    ? 1
    : stepImageZoom(current, 1, geometry);
}
