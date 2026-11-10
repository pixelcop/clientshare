import { flushPromises } from '@vue/test-utils';
import axios from 'axios';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { effectScope, nextTick } from 'vue';

import { useWorkspacePresentation } from '@/composables/useWorkspacePresentation';
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

  it('retains the settled view during navigation without retaining authority to mutate it', async () => {
    const store = useFileManagerStore();
    await store.changeFolder('old');
    await flushPromises();
    store.files = [{ ...folder('document'), type: 'file' }];
    const scope = effectScope();
    const presentation = scope.run(useWorkspacePresentation)!;
    let finish!: () => void;
    const pending = new Promise<void>((resolve) => {
      finish = resolve;
    });
    vi.mocked(axios.get).mockImplementation(async (url, config) => {
      await pending;
      return response(String(url), config);
    });
    const post = vi.spyOn(axios, 'post');

    await store.changeFolder('new');
    await nextTick();
    expect(store.context).toBeNull();
    expect(store.breadcrumbs).toEqual([]);
    expect(store.canWrite).toBe(false);
    expect(presentation.value.context?.folder.id).toBe('old');
    expect(presentation.value.files[0].id).toBe('document');
    expect(presentation.value.breadcrumbs[0].id).toBe('old');
    expect(await store.createFolder('Not allowed')).toBeNull();
    expect(post).not.toHaveBeenCalled();

    finish();
    await flushPromises();
    expect(presentation.value.context?.folder.id).toBe('new');
    expect(presentation.value.breadcrumbs[0].id).toBe('new');
    expect(presentation.value.files).toEqual([]);
    scope.stop();
  });

  it('discards the presentation after a failed destination request', async () => {
    const store = useFileManagerStore();
    await store.changeFolder('old');
    await flushPromises();
    const scope = effectScope();
    const presentation = scope.run(useWorkspacePresentation)!;
    vi.mocked(axios.get).mockRejectedValue(new Error('Access revoked'));
    await store.changeFolder('denied');
    await flushPromises();
    expect(presentation.value.context).toBeNull();
    expect(presentation.value.breadcrumbs).toEqual([]);
    expect(presentation.value.loaded).toBe(false);
    scope.stop();
  });

  it.each(['id', 'tenant_id', 'role', 'session_id', 'link_id'])(
    'immediately clears the presentation when %s changes during a pending navigation',
    async (field) => {
      const store = useFileManagerStore();
      await store.changeFolder('old');
      await flushPromises();
      const scope = effectScope();
      const presentation = scope.run(useWorkspacePresentation)!;
      let finish!: () => void;
      const pending = new Promise<void>((resolve) => {
        finish = resolve;
      });
      vi.mocked(axios.get).mockImplementation(async (url, config) => {
        await pending;
        return response(String(url), config);
      });
      await store.changeFolder('new');
      await nextTick();
      expect(presentation.value.context?.folder.id).toBe('old');
      const auth = useAuthStore();
      auth.user = { ...auth.user!, [field]: 'changed' };
      expect(presentation.value.context).toBeNull();
      expect(presentation.value.breadcrumbs).toEqual([]);
      finish();
      await flushPromises();
      expect(presentation.value.context).toBeNull();
      scope.stop();
    },
  );

  it('preserves the workspace when auth/me refreshes the same signed-in identity', async () => {
    const auth = useAuthStore();
    auth.user = { ...auth.user!, session_id: 'same-session', tenant_id: 'tenant' };
    const store = useFileManagerStore();
    await store.changeFolder('root');
    await flushPromises();
    store.files = [{ ...folder('document'), type: 'file' }];
    store.folders = [folder('child')];
    const { context, breadcrumbs, files, folders } = store;
    store.sortField = 'filename';
    vi.mocked(axios.get).mockResolvedValueOnce({ data: { ...auth.user, name: 'Updated name' } });
    await auth.fetchMe(true);
    await flushPromises();
    expect(auth.user?.name).toBe('Updated name');
    expect(store.currentFolder).toBe('root');
    expect(store.context).toBe(context);
    expect(store.breadcrumbs).toBe(breadcrumbs);
    expect(store.files).toBe(files);
    expect(store.folders).toBe(folders);
    expect(store.clientId).toBe('client-root');
    expect(store.loaded).toBe(true);
    expect(store.sortField).toBe('filename');
  });

  it.each(['id', 'tenant_id', 'role', 'session_id', 'link_id'])(
    'still clears protected workspace data when %s changes',
    async (field) => {
      const auth = useAuthStore();
      const store = useFileManagerStore();
      await store.changeFolder('root');
      await flushPromises();
      auth.user = { ...auth.user!, [field]: 'changed' };
      expect(store.currentFolder).toBeNull();
      expect(store.context).toBeNull();
      expect(store.clientId).toBeNull();
      expect(store.loaded).toBe(false);
    },
  );

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
