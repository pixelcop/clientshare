import axios from 'axios';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { nextTick } from 'vue';

import { useAuthStore } from '@/stores/auth';
import type { CollectionLink } from '@/stores/collection';
import {
  collectionStep,
  guideTopic,
  usableUploadLink,
  useCollectionStore,
} from '@/stores/collection';
import type { Client, User } from '@/types/models';

const client = { id: 'client-1' } as Client;
const link: CollectionLink = {
  id: 'link-1',
  access_type: 'write',
  created_at: new Date().toISOString(),
  expires_at: new Date(Date.now() + 3600000).toISOString(),
  received: false,
};
describe('document collection', () => {
  beforeEach(() => {
    localStorage.clear();
    setActivePinia(createPinia());
    vi.restoreAllMocks();
  });
  it('separates selection, preparation, copying and receipt; invalidates expired or revoked links', () => {
    expect(collectionStep(null, undefined)).toBe(0);
    expect(collectionStep(client, undefined, 'copied')).toBe(1);
    expect(collectionStep(client, link)).toBe(2);
    expect(collectionStep(client, link, 'copied')).toBe(3);
    expect(collectionStep(client, { ...link, received: true })).toBe(4);
    expect(collectionStep(client, { ...link, expires_at: '2000-01-01' }, 'copied')).toBe(1);
    expect(usableUploadLink({ ...link, access_type: 'read' })).toBe(false);
  });
  it('resolves bundled and hosted guide paths', () => {
    expect(new URL(guideTopic('/docs/', 'sharing/create-link')).pathname).toBe(
      '/docs/sharing/create-link.html',
    );
    expect(guideTopic('https://guide.example.com/manual', 'clients/manage-clients')).toBe(
      'https://guide.example.com/manual/clients/manage-clients.html',
    );
    expect(
      guideTopic('https://guide.example.com/index.html#intro', 'activity/review-activity'),
    ).toBe('https://guide.example.com/activity/review-activity.html');
  });
  it.each(['manager', 'client', 'link'])('does not initiate or query for %s', async (role) => {
    useAuthStore().user = { id: 'u', tenant_id: 't', role } as User;
    const get = vi.spyOn(axios, 'get');
    const store = useCollectionStore();
    store.start('client-1');
    await nextTick();
    expect(store.progress).toBeNull();
    expect(get).not.toHaveBeenCalled();
  });
  it('reconciles outside actions, resumes, handles deletion and keeps tokens out of persisted context', async () => {
    useAuthStore().user = { id: 'u', tenant_id: 't', role: 'admin' } as User;
    const get = vi.spyOn(axios, 'get').mockImplementation(async (url) => ({
      data: String(url).includes('/collection')
        ? [{ ...link, created_at: new Date(Date.now() + 1000).toISOString() }]
        : client,
    }));
    const store = useCollectionStore();
    store.start('client-1');
    await store.reconcile();
    expect(store.step).toBe(2);
    store.shared('client-1', 'link-1', 'copied');
    await store.reconcile();
    expect(store.step).toBe(3);
    store.save({ dismissed: true });
    store.start();
    await store.reconcile();
    expect(store.progress?.shared).toBe('copied');
    expect(store.progress?.dismissed).toBe(false);
    useAuthStore().user!.role = 'manager';
    await nextTick();
    expect(store.allowed).toBe(false);
    expect(store.client).toBeNull();
    useAuthStore().user!.role = 'admin';
    await nextTick();
    await store.reconcile();
    expect(
      Object.keys(JSON.parse(localStorage.getItem('clientshare:collect:v1:t:u')!)).sort(),
    ).toEqual(['clientId', 'collapsed', 'dismissed', 'linkId', 'shared', 'started']);
    get.mockRejectedValue(
      new axios.AxiosError('missing', '404', undefined, undefined, { status: 404 } as never),
    );
    await store.reconcile();
    expect(store.progress?.clientId).toBeUndefined();
    expect(store.step).toBe(0);
  });
});
