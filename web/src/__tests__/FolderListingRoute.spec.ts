import { flushPromises, mount } from '@vue/test-utils';
import axios from 'axios';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h, watch } from 'vue';
import { createMemoryHistory, createRouter, useRoute } from 'vue-router';

import { useFolderListingRoute } from '@/composables/useFolderListingRoute';
import { useAuthStore } from '@/stores/auth';
import { useFileManagerStore } from '@/stores/fileManager';
import type { User } from '@/types/models';
import { fileListingQuery, parseFileListingQuery } from '@/utils/fileListingQuery';

describe('folder listing URLs', () => {
  let total: number;
  beforeEach(() => {
    vi.restoreAllMocks();
    setActivePinia(createPinia());
    useAuthStore().user = { id: 'user', role: 'admin' } as User;
    total = 57;
    vi.spyOn(axios, 'get').mockImplementation(async (url, config) => {
      if (String(url).endsWith('/breadcrumbs')) {
        return { data: [] };
      }
      const pageSize = config?.params?.page_size || 20;
      const totalPages = Math.ceil(total / pageSize);
      const page = Math.max(1, Math.min(config?.params?.page || 1, totalPages));
      return {
        data: {
          items: [],
          pagination: { page, page_size: pageSize, total_items: total, total_pages: totalPages },
          context: { folder: { id: 'root' }, client_id: 'client', capabilities: {} },
        },
      };
    });
  });

  async function render(url: string) {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/folder/:folderId', component: { template: '<div />' } }],
    });
    await router.push(url);
    const harness = defineComponent({
      setup() {
        const route = useRoute();
        const store = useFileManagerStore();
        useFolderListingRoute();
        watch(
          () => route.params.folderId,
          (id) => {
            void store.changeFolder(String(id));
          },
          { immediate: true },
        );
        return () => h('div');
      },
    });
    const wrapper = mount(harness, { global: { plugins: [router] } });
    await flushPromises();
    return { router, wrapper, store: useFileManagerStore() };
  }

  it('loads bookmarked controls in a single unified listing request', async () => {
    const { wrapper, store } = await render(
      '/folder/root?page=3&page_size=10&sort_by=filename&sort_dir=asc',
    );
    expect(axios.get).toHaveBeenCalledWith('/api/folders/root', {
      params: { type: 'all', page: 3, page_size: 10, sort_by: 'filename', sort_dir: 'asc' },
    });
    expect(
      vi.mocked(axios.get).mock.calls.filter(([url]) => url === '/api/folders/root'),
    ).toHaveLength(1);
    expect(store.pagination.page).toBe(3);
    expect(store.sortField).toBe('filename');
    wrapper.unmount();
  });

  it('restores page, size and sorting on Back/Forward and resets controls for a new folder', async () => {
    const { router, wrapper, store } = await render(
      '/folder/root?page=2&page_size=10&sort_by=filename&sort_dir=asc',
    );
    await router.push('/folder/root?page=4&page_size=10&sort_by=size');
    await flushPromises();
    expect(store.pagination.page).toBe(4);
    expect(store.sortField).toBe('size');
    router.back();
    await vi.waitFor(() => expect(store.pagination.page).toBe(2));
    expect(store.sortField).toBe('filename');
    expect(store.sortDirection).toBe('asc');
    router.forward();
    await vi.waitFor(() => expect(store.pagination.page).toBe(4));
    expect(store.sortField).toBe('size');
    await router.push('/folder/child');
    await flushPromises();
    expect(store.pagination.page).toBe(1);
    expect(store.pagination.page_size).toBe(20);
    expect(store.sortField).toBe('uploaded_at');
    wrapper.unmount();
  });

  it('replaces a vanished page with the last page without adding an extra history step', async () => {
    const { router, wrapper, store } = await render('/folder/root?page=2&keep=yes');
    await router.push('/folder/root?page=99&keep=yes');
    await flushPromises();
    expect(store.pagination.page).toBe(3);
    expect(router.currentRoute.value.query).toEqual({ page: '3', keep: 'yes' });
    router.back();
    await vi.waitFor(() => expect(store.pagination.page).toBe(2));
    total = 0;
    await store.fetchFiles();
    await flushPromises();
    expect(store.pagination.page).toBe(1);
    expect(router.currentRoute.value.query).toEqual({ keep: 'yes' });
    wrapper.unmount();
  });

  it('normalizes invalid controls and preserves unrelated query parameters', async () => {
    const { router, wrapper } = await render(
      '/folder/root?page=-2&page_size=999&sort_by=invalid&sort_dir=invalid&keep=yes',
    );
    expect(router.currentRoute.value.query).toEqual({ keep: 'yes' });
    wrapper.unmount();
    expect(
      parseFileListingQuery({ page: ['2', '3'], sort_by: ['filename'], page_size: '10' }),
    ).toEqual({ page: 1, pageSize: 10, sortField: 'uploaded_at', sortDirection: 'desc' });
    expect(
      fileListingQuery({ page: 2, pageSize: 50, sortField: 'size', sortDirection: 'asc' }),
    ).toEqual({ page: '2', page_size: '50', sort_by: 'size', sort_dir: 'asc' });
  });

  it('does not let a slower previous page overwrite a newer URL or result', async () => {
    const { router, wrapper, store } = await render('/folder/root');
    const original = vi.mocked(axios.get).getMockImplementation()!;
    let finish!: () => void;
    vi.mocked(axios.get).mockImplementation(async (url, config) => {
      const result = await original(url, config);
      if (config?.params?.page === 2) {
        return new Promise((resolve) => {
          finish = () => resolve(result);
        });
      }
      return result;
    });
    await router.push('/folder/root?page=2');
    await flushPromises();
    await router.push('/folder/root?page=3');
    await flushPromises();
    expect(store.pagination.page).toBe(3);
    finish();
    await flushPromises();
    expect(store.pagination.page).toBe(3);
    expect(router.currentRoute.value.query.page).toBe('3');
    wrapper.unmount();
  });
});
