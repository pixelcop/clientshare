/** Keep the longest fitting suffix, always retaining the current location. */
export function breadcrumbHiddenCount(
  widths: number[],
  available: number,
  overflow: number,
): number {
  let total = widths.reduce((sum, width) => sum + width, 0);
  if (total <= available) {
    return 0;
  }
  for (let hidden = 1; hidden < widths.length; hidden += 1) {
    total -= widths[hidden - 1];
    if (total + overflow <= available) {
      return hidden;
    }
  }
  return Math.max(0, widths.length - 1);
}
