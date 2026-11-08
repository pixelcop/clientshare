import { flushPromises } from '@vue/test-utils';
import axios from 'axios';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useAuthStore } from '@/stores/auth';
import { useFileManagerStore } from '@/stores/fileManager';
import type { FileItem, User } from '@/types/models';
import { defaultPagination } from '@/types/util';

const folder = (id: string) =>
  ({ id, client_id: `client-${id}`, type: 'folder', filename: id }) as FileItem;
const listing = (id: string, items: FileItem[] = []) => ({
  items,
  pagination: { ...defaultPagination(), total_items: items.length },
  context: {
    folder: folder(id),
    client_id: `client-${id}`,
    client_name: id,
    root_folder_id: id,
    capabilities: { can_read: true, can_upload: true, can_delete: true, can_share: true },
    client_capabilities: { can_read: true, can_upload: true, can_delete: true, can_share: true },
    can_manage_client: true,
    can_manage_queue: true,
  },
});

function response(url: string, config?: { params?: { type?: string } }) {
  if (url === '/api/clients') {
    return { data: [] };
  }
  const id = url.split('/')[3];
  if (url.endsWith('/breadcrumbs')) {
    return { data: [folder(id)] };
  }
  return { data: listing(id, config?.params?.type === 'folder' ? [] : []) };
}

describe('workspace listing state', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setActivePinia(createPinia());
    useAuthStore().user = { id: 'user', role: 'admin' } as User;
    vi.spyOn(axios, 'get').mockImplementation(async (url, config) => response(String(url), config));
  });

  it('establishes explicit client context for an empty root', async () => {
    const store = useFileManagerStore();
    await store.changeFolder('empty');
    await flushPromises();
    expect(store.context?.folder.id).toBe('empty');
    expect(store.clientId).toBe('client-empty');
    expect(store.isEmpty).toBe(true);
  });

  it('waits for both lists and does not treat folders-only content as empty', async () => {
    let finish!: (value: unknown) => void;
    vi.mocked(axios.get).mockImplementation(async (url, config) => {
      if (config?.params?.type === 'folder') {
        return new Promise((resolve) => {
          finish = resolve;
        });
      }
      return response(String(url), config);
    });
    const store = useFileManagerStore();
    await store.changeFolder('root');
    await flushPromises();
    expect(store.loading).toBe(true);
    expect(store.isEmpty).toBe(false);
    finish({ data: listing('root', [folder('child')]) });
    await flushPromises();
    expect(store.loading).toBe(false);
    expect(store.folders).toHaveLength(1);
    expect(store.isEmpty).toBe(false);
  });

  it.each(['files', 'folders', 'breadcrumbs'])(
    'surfaces a failed %s query, never an empty state',
    async (surface) => {
      vi.mocked(axios.get).mockImplementation(async (url, config) => {
        if (
          String(url) !== '/api/clients' &&
          ((surface === 'breadcrumbs' && String(url).endsWith('/breadcrumbs')) ||
            (surface === 'folders' && config?.params?.type === 'folder') ||
            (surface === 'files' && !config?.params?.type && !String(url).endsWith('/breadcrumbs')))
        ) {
          throw Object.assign(new Error('Query failed'), {
            response: { data: { error: `${surface} unavailable` } },
          });
        }
        return response(String(url), config);
      });
      const store = useFileManagerStore();
      await store.changeFolder('root');
      await flushPromises();
      expect(store.error).toBe(`${surface} unavailable`);
      expect(store.isEmpty).toBe(false);
      expect(store.loaded).toBe(false);
    },
  );

  it('discards late responses and errors after a folder switch', async () => {
    let finish!: (value: unknown) => void;
    vi.mocked(axios.get).mockImplementation(async (url, config) => {
      if (String(url) === '/api/folders/old' && !config?.params?.type) {
        return new Promise((resolve) => {
          finish = resolve;
        });
      }
      return response(String(url), config);
    });
    const store = useFileManagerStore();
    await store.changeFolder('old');
    void store.changeFolder('new');
    expect(store.context).toBeNull();
    await flushPromises();
    finish({ data: listing('old', [folder('private')]) });
    await flushPromises();
    expect(store.context?.folder.id).toBe('new');
    expect(store.clientId).toBe('client-new');
    expect(store.files).toEqual([]);
    expect(store.error).toBeNull();
  });

  it('backs up from an emptied final pagination page', async () => {
    const get = vi.mocked(axios.get);
    get.mockImplementation(async (url, config) => {
      const result = response(String(url), config);
      if (String(url) === '/api/folders/root' && !config?.params?.type) {
        return {
          data: {
            ...listing('root', config?.params?.page === 2 ? [] : [folder('file')]),
            pagination: { ...defaultPagination(), total_items: 1, total_pages: 1 },
          },
        };
      }
      return result;
    });
    const store = useFileManagerStore();
    await store.changeFolder('root');
    await flushPromises();
    await store.fetchFiles(2, 20);
    expect(store.files[0]?.id).toBe('file');
    expect(store.isEmpty).toBe(false);
    expect(store.pagination.page).toBe(1);
  });

  it('does not infer parent authorization from single-file metadata', async () => {
    const store = useFileManagerStore();
    vi.mocked(axios.get).mockResolvedValue({
      data: { ...folder('file'), folder_id: 'private-parent' },
    });
    await store.fetchFile('file');
    expect(store.currentFolder).toBeNull();
    expect(store.clientId).toBeNull();
  });
});
