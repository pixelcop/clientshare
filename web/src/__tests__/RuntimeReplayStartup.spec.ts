import { afterEach, expect, it, vi } from 'vitest';

import { refreshRuntimeReplay, startRuntimeReplay } from '../services/runtimeReplay';

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

it('makes one startup config request, coalesces refreshes and refreshes on restored pages', async () => {
  vi.useFakeTimers();
  const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ debug: { replay: { enabled: false } } }) });
  vi.stubGlobal('fetch', fetch);
  startRuntimeReplay();
  window.dispatchEvent(new PageTransitionEvent('pageshow', { persisted: false }));
  await Promise.all([refreshRuntimeReplay(), refreshRuntimeReplay()]);
  expect(fetch).toHaveBeenCalledTimes(1);
  window.dispatchEvent(new PageTransitionEvent('pageshow', { persisted: true }));
  await refreshRuntimeReplay();
  expect(fetch).toHaveBeenCalledTimes(2);
  await vi.advanceTimersByTimeAsync(30_000);
  expect(fetch).toHaveBeenCalledTimes(3);
});
