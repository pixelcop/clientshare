import { describe, expect, it } from 'vitest';

import { breadcrumbHiddenCount } from '@/utils/breadcrumb';

describe('breadcrumb overflow', () => {
  it('uses all available width without a folder-depth limit', () => {
    expect(breadcrumbHiddenCount([60, 80, 70, 90, 50, 80], 430, 40)).toBe(0);
  });

  it('hides only the oldest folders needed to fit, including the ellipsis', () => {
    expect(breadcrumbHiddenCount([60, 80, 70, 90], 240, 40)).toBe(2);
    expect(breadcrumbHiddenCount([60, 80, 70, 90], 280, 40)).toBe(1);
  });

  it('always retains the current folder, even with a very long name', () => {
    expect(breadcrumbHiddenCount([60, 80, 900], 100, 40)).toBe(2);
    expect(breadcrumbHiddenCount([900], 100, 40)).toBe(0);
  });

  it('supports root folders and resizing back to the full hierarchy', () => {
    expect(breadcrumbHiddenCount([], 100, 40)).toBe(0);
    expect(breadcrumbHiddenCount([80, 80, 80], 140, 40)).toBe(2);
    expect(breadcrumbHiddenCount([80, 80, 80], 240, 40)).toBe(0);
  });
});
