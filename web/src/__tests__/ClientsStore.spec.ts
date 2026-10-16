import { flushPromises } from '@vue/test-utils';
import axios from 'axios';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useClientsStore } from '../stores/clients';
import type { Client } from '../types/models';

describe('client list loading', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  afterEach(() => vi.restoreAllMocks());

  it('shares an in-flight request and allows refreshing after it completes', async () => {
    let resolveRequest!: (response: { data: Client[] }) => void;
    const get = vi.spyOn(axios, 'get').mockImplementationOnce(
      () => new Promise((resolve) => {
        resolveRequest = resolve;
      }),
    );
    const store = useClientsStore();
    const first = store.fetchClients();
    const second = store.fetchClients();

    expect(get).toHaveBeenCalledTimes(1);
    resolveRequest({ data: [{ id: 'client-1', name: 'First client' } as Client] });
    await Promise.all([first, second]);
    expect(store.clients.map((client) => client.name)).toEqual(['First client']);

    get.mockResolvedValueOnce({ data: [] });
    await store.fetchClients();
    expect(get).toHaveBeenCalledTimes(2);
    expect(store.clients).toEqual([]);
  });

  it('settles failed requests and allows a successful retry', async () => {
    const get = vi.spyOn(axios, 'get').mockRejectedValueOnce(new Error('Unavailable'));
    const store = useClientsStore();
    await store.fetchClients();
    await flushPromises();

    expect(store.error).toBe('Failed to fetch clients');
    get.mockResolvedValueOnce({ data: [{ id: 'client-2', name: 'Retry client' }] });
    await store.fetchClients();

    expect(get).toHaveBeenCalledTimes(2);
    expect(store.error).toBeNull();
    expect(store.clients.map((client) => client.name)).toEqual(['Retry client']);
  });
});
