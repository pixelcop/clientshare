import { flushPromises, mount } from '@vue/test-utils';
import axios from 'axios';
import { SplitButton } from 'openvue';
import OpenVue from 'openvue/config';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';

import Dropzone from '@/components/Dropzone.vue';
import { useAuthStore } from '@/stores/auth';
import { useFileManagerStore } from '@/stores/fileManager';
import type { FileItem, User } from '@/types/models';

beforeEach(() => {
  setActivePinia(createPinia());
  useAuthStore().user = { id: 'uploader', role: 'client' } as User;
  vi.spyOn(axios, 'get').mockResolvedValue({ data: [] });
  vi.stubGlobal(
    'matchMedia',
    vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })),
  );
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it('binds file and directory chooser refs, including after authorization changes', async () => {
  const files = useFileManagerStore();
  files.currentFolder = 'folder';
  await flushPromises();
  const caps = { can_read: true, can_upload: true, can_delete: false, can_share: false };
  files.context = {
    folder: { id: 'folder', filename: 'Folder', type: 'folder' } as FileItem,
    client_id: 'client',
    client_name: 'Client',
    root_folder_id: 'folder',
    capabilities: caps,
    client_capabilities: caps,
    can_manage_client: false,
    can_manage_queue: false,
  };
  files.loaded = true;
  const wrapper = mount(Dropzone, {
    props: { canNotify: false },
    global: { plugins: [OpenVue], stubs: { PendingEmail: true, NewFolderModal: true } },
  });
  await flushPromises();
  const file = wrapper.get('input[aria-label="Upload files"]').element as HTMLInputElement;
  const directory = wrapper.get('input[aria-label="Upload folder"]').element as HTMLInputElement;
  const fileClick = vi.spyOn(file, 'click').mockImplementation(() => undefined);
  const directoryClick = vi.spyOn(directory, 'click').mockImplementation(() => undefined);
  wrapper.getComponent(SplitButton).vm.$emit('click');
  const uploadFolder = wrapper
    .getComponent(SplitButton)
    .props('model')
    .find((item) => item.label === 'Upload folder')!;
  uploadFolder.command!({ originalEvent: new Event('click'), item: uploadFolder });
  expect(fileClick).toHaveBeenCalledOnce();
  expect(directoryClick).toHaveBeenCalledOnce();
  files.context.capabilities.can_upload = false;
  await flushPromises();
  expect(wrapper.find('input[aria-label="Upload files"]').exists()).toBe(false);
  files.context.capabilities.can_upload = true;
  await flushPromises();
  const newFile = wrapper.get('input[aria-label="Upload files"]').element as HTMLInputElement;
  const newClick = vi.spyOn(newFile, 'click').mockImplementation(() => undefined);
  wrapper.getComponent(SplitButton).vm.$emit('click');
  expect(newClick).toHaveBeenCalledOnce();
  expect(fileClick).toHaveBeenCalledOnce();
  wrapper.unmount();
});
