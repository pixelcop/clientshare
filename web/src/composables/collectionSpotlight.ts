export interface SpotlightRect {
  left: number;
  top: number;
  width: number;
  height: number;
}
export interface SpotlightViewport {
  width: number;
  height: number;
  top?: number;
  left?: number;
}

// Keep the coach outside the actual control, including on narrow/keyboard-sized viewports.
export function placeCollectionCoach(
  target: SpotlightRect,
  viewport: SpotlightViewport,
  height = 164,
) {
  const margin = 12;
  const gap = 20;
  const left = (viewport.left || 0) + margin;
  const top = (viewport.top || 0) + margin;
  const right = left + viewport.width - margin * 2;
  const bottom = top + viewport.height - margin * 2;
  const width = Math.min(296, right - left);
  const clamp = (n: number, min: number, max: number) => Math.max(min, Math.min(n, max));
  const x = clamp(target.left, left, right - width);
  const y = clamp(target.top, top, bottom - height);
  const options = [
    { side: 'left', x: target.left - width - gap, y },
    { side: 'right', x: target.left + target.width + gap, y },
    { side: 'above', x, y: target.top - height - gap },
    { side: 'below', x, y: target.top + target.height + gap },
  ];
  const placement = options.find(
    (p) => p.x >= left && p.y >= top && p.x + width <= right && p.y + height <= bottom,
  );
  return placement ? { ...placement, width } : null;
}
