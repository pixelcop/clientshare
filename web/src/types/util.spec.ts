import { afterEach, describe, expect, it, vi } from 'vitest';

import { formatDateNatural } from './util';

describe('formatDateNatural', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('preserves the placeholder for missing dates', () => {
    expect(formatDateNatural('')).toBe('-');
  });

  it.each([
    [19 * 60_000, -19, 'minute'],
    [9 * 30 * 86_400_000, -9, 'month'],
    [86_400_000, -1, 'day'],
    [7 * 86_400_000, -1, 'week'],
    [365 * 86_400_000, -1, 'year'],
    [-19 * 60_000, 19, 'minute'],
    [0, 0, 'second'],
  ] as const)('formats a %i ms distance with %i %s', (distance, value, unit) => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-11-10T12:00:00Z'));
    const formatter = new Intl.RelativeTimeFormat(undefined, {
      style: 'narrow',
      numeric: 'auto',
    });

    expect(formatDateNatural(new Date(Date.now() - distance).toISOString())).toBe(
      formatter.format(value, unit),
    );
  });
});
