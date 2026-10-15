import { flushPromises } from '@vue/test-utils';
import axios from 'axios';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useBrandingStore } from '../stores/branding';

describe('runtime documentation URL', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  afterEach(() => vi.restoreAllMocks());

  it.each(['/docs/', 'https://docs.clientshare.app', 'https://help.example.com/guide/'])(
    'uses the server-provided URL %s',
    async (docsURL) => {
      vi.spyOn(axios, 'get').mockResolvedValue({ data: { docs_url: docsURL } });
      const store = useBrandingStore();
      await flushPromises();
      expect(store.branding.docsURL).toBe(docsURL);
    },
  );

  it('falls back to local docs if branding cannot load', async () => {
    vi.spyOn(axios, 'get').mockRejectedValue(new Error('Unavailable'));
    const store = useBrandingStore();
    await flushPromises();
    expect(store.branding.docsURL).toBe('/docs/');
  });
});
