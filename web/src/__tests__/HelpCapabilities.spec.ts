import axios from 'axios';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { fetchHelpCapabilities, invalidateHelpCapabilities } from '../services/helpCapabilities';

describe('shared help capabilities', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    invalidateHelpCapabilities();
  });

  it('shares concurrent consumers and caches later Help opens', async () => {
    const get = vi.spyOn(axios, 'get').mockResolvedValue({ data: { provider: 'clientshare' } });
    const first = fetchHelpCapabilities();
    expect(fetchHelpCapabilities()).toBe(first);
    await first;
    await fetchHelpCapabilities();
    expect(get).toHaveBeenCalledExactlyOnceWith('/api/help');
    invalidateHelpCapabilities();
    await fetchHelpCapabilities();
    expect(get).toHaveBeenCalledTimes(2);
  });

  it('retries failed requests', async () => {
    const get = vi.spyOn(axios, 'get').mockRejectedValueOnce(new Error('offline'));
    await expect(fetchHelpCapabilities()).rejects.toThrow('offline');
    get.mockResolvedValue({ data: { provider: 'none' } });
    await expect(fetchHelpCapabilities()).resolves.toEqual({ provider: 'none' });
    expect(get).toHaveBeenCalledTimes(2);
  });

  it('does not let an old session’s failure evict a newer request', async () => {
    let reject!: (error: Error) => void;
    const get = vi.spyOn(axios, 'get').mockImplementationOnce(() => new Promise((resolve, fail) => {
      reject = fail;
    }));
    const old = fetchHelpCapabilities();
    invalidateHelpCapabilities();
    get.mockResolvedValue({ data: { provider: 'clientshare' } });
    const current = fetchHelpCapabilities();
    reject(new Error('Old session ended'));
    await expect(old).rejects.toThrow('Old session ended');
    expect(fetchHelpCapabilities()).toBe(current);
    await current;
    expect(get).toHaveBeenCalledTimes(2);
  });
});
