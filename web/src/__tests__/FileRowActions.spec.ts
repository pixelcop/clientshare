import { mount } from '@vue/test-utils';
import axios from 'axios';
import OpenVue from 'openvue/config';
import Menu from 'openvue/menu';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { nextTick } from 'vue';

import ConfirmDeleteModal from '@/components/ConfirmDeleteModal.vue';
import FileListItem from '@/components/FileListItem.vue';
import FileRowActions from '@/components/FileRowActions.vue';
import { useAuthStore } from '@/stores/auth';
import { useFileManagerStore } from '@/stores/fileManager';
import type { FileItem, User } from '@/types/models';

const item = {
  id: 'file',
  type: 'file',
  filename: 'report.txt',
  uploaded_at: new Date().toISOString(),
  capabilities: { can_read: true, can_upload: false, can_share: false, can_delete: true },
} as FileItem;

describe('file row actions', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setActivePinia(createPinia());
    vi.spyOn(axios, 'get').mockResolvedValue({ data: [] });
    useAuthStore().user = { id: 'admin', role: 'admin' } as User;
  });

  it('activates preview exactly once and keeps Delete separate in More', async () => {
    const wrapper = mount(FileListItem, { props: { item }, global: { plugins: [OpenVue] } });
    await wrapper.get('button[aria-label="Preview report.txt"]').trigger('click');
    expect(wrapper.emitted('open')).toEqual([[item]]);
    const actions = wrapper.findComponent(Menu).props('model')!;
    expect(actions.map((action) => action.label || 'separator')).toEqual([
      'Preview',
      'Download',
      'separator',
      'Delete',
    ]);
    expect(actions.some((action) => action.label === 'Share')).toBe(false);
    wrapper.unmount();
  });

  it('downloads unsupported preview types instead of promising a preview', async () => {
    const store = useFileManagerStore();
    const download = vi.spyOn(store, 'downloadFile').mockResolvedValue();
    const document = { ...item, filename: 'archive.zip' };
    const wrapper = mount(FileRowActions, {
      props: { item: document },
      global: { plugins: [OpenVue] },
    });
    expect(wrapper.find('button[aria-label^="Preview"]').exists()).toBe(false);
    await wrapper.get('button[aria-label="Download archive.zip"]').trigger('click');
    expect(download).toHaveBeenCalledExactlyOnceWith(document);
    expect(wrapper.emitted('open')).toBeUndefined();
    wrapper.unmount();
  });

  it('closes stale row dialogs when navigation starts while the row remains displayed', async () => {
    const store = useFileManagerStore();
    const wrapper = mount(FileRowActions, {
      props: { item },
      global: { plugins: [OpenVue], stubs: { ConfirmDeleteModal: true } },
    });
    const deleteAction = wrapper
      .findComponent(Menu)
      .props('model')!
      .find((action) => action.label === 'Delete')!;
    deleteAction.command?.({ originalEvent: new Event('click'), item: deleteAction });
    await nextTick();
    expect(wrapper.findComponent(ConfirmDeleteModal).exists()).toBe(true);
    store.loading = true;
    await nextTick();
    expect(wrapper.findComponent(ConfirmDeleteModal).exists()).toBe(false);
    wrapper.unmount();
  });

  it('does not offer Delete to a client account and exposes folder Open/ZIP', () => {
    useAuthStore().user = { id: 'client', role: 'client' } as User;
    const wrapper = mount(FileRowActions, {
      props: {
        item: {
          ...item,
          type: 'folder',
          filename: 'Documents',
          capabilities: { ...item.capabilities!, can_delete: false },
        },
      },
      global: { plugins: [OpenVue] },
    });
    const actions = wrapper.findComponent(Menu).props('model')!;
    expect(actions.map((action) => action.label)).toEqual(['Open', 'Download ZIP']);
    expect(wrapper.find('button[aria-label="Open Documents"]').exists()).toBe(true);
    wrapper.unmount();
  });
});
