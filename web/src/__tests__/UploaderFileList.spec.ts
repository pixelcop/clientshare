import { mount } from '@vue/test-utils';
import axios from 'axios';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, expect, it, vi } from 'vitest';
import { createMemoryHistory, createRouter } from 'vue-router';

import FileList from '@/components/FileList.vue';
import FileListItem from '@/components/FileListItem.vue';
import { useAuthStore } from '@/stores/auth';
import { useFileManagerStore } from '@/stores/fileManager';
import type { FileItem, User } from '@/types/models';
import { defaultPagination } from '@/types/util';

beforeEach(() => {
  setActivePinia(createPinia());
  vi.spyOn(axios, 'get').mockResolvedValue({ data: [] });
  useAuthStore().user = { id: 'uploader', role: 'client' } as User;
});

it('shows and opens authorized own items inside an upload-only destination', async () => {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }],
  });
  await router.push('/folder/root');
  const store = useFileManagerStore();
  const caps = { can_read: false, can_upload: true, can_delete: false, can_share: false };
  const item = {
    id: 'own-folder',
    type: 'folder',
    filename: 'My folder',
    capabilities: { ...caps, can_read: true },
  } as FileItem;
  store.context = {
    folder: { id: 'root', type: 'folder' } as FileItem,
    client_id: 'client',
    client_name: 'Client',
    root_folder_id: 'root',
    capabilities: caps,
    client_capabilities: caps,
    can_manage_client: false,
    can_manage_queue: false,
  };
  store.items = [item];
  store.pagination = { ...defaultPagination(), total_items: 1 };
  store.loaded = true;
  const wrapper = mount(FileList, {
    global: {
      plugins: [router],
      stubs: {
        Dropzone: { template: '<div><slot /></div>' },
        FileListItem: true,
        Paginator: true,
      },
    },
  });
  expect(wrapper.find('table').exists()).toBe(true);
  expect(wrapper.find('.workspace-empty').exists()).toBe(false);
  wrapper.findComponent(FileListItem).vm.$emit('open', item);
  await vi.waitFor(() => expect(router.currentRoute.value.path).toBe('/folder/own-folder'));
  wrapper.unmount();
});

it.each(['report.pdf', 'archive.zip'])(
  'opens %s at its own URL without downloading',
  async (filename) => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/folder/:folderId', component: { template: '<div />' } },
        { path: '/files/:fileId', name: 'FileView', component: { template: '<div />' } },
      ],
    });
    await router.push('/folder/root');
    const store = useFileManagerStore();
    const download = vi.spyOn(store, 'downloadFile').mockResolvedValue();
    const item = {
      id: 'file',
      filename,
      type: 'file',
      capabilities: { can_read: true },
    } as FileItem;
    store.context = {
      folder: { id: 'root', type: 'folder' } as FileItem,
      capabilities: { can_read: true },
    } as typeof store.context;
    store.items = [item];
    store.loaded = true;
    const wrapper = mount(FileList, {
      global: {
        plugins: [router],
        stubs: {
          Dropzone: { template: '<div><slot /></div>' },
          FileListItem: true,
          Paginator: true,
        },
      },
    });
    wrapper.findComponent(FileListItem).vm.$emit('open', item);
    await vi.waitFor(() => expect(router.currentRoute.value.path).toBe('/files/file'));
    expect(download).not.toHaveBeenCalled();
    wrapper.unmount();
  },
);
