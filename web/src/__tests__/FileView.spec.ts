import { flushPromises, mount } from '@vue/test-utils';
import axios from 'axios';
import OpenVue from 'openvue/config';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createMemoryHistory, createRouter } from 'vue-router';

import FilePreview from '@/components/FilePreview.vue';
import { useAuthStore } from '@/stores/auth';
import { useFileManagerStore } from '@/stores/fileManager';
import type { FileItem, User } from '@/types/models';
import FileView from '@/views/FileView.vue';

const file = {
  id: 'file',
  filename: 'archive.zip',
  type: 'file',
  folder_id: 'folder',
  size: 4096,
  uploaded_at: '2026-11-13T12:00:00Z',
  capabilities: { can_read: true, can_share: true },
} as FileItem;

async function render(previous?: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/files/:fileId', name: 'FileView', component: FileView, meta: { public: false } },
      {
        path: '/folder/:folderId',
        name: 'FolderView',
        component: { template: '<div />' },
        meta: { public: false },
      },
      {
        path: '/shared-with-me',
        name: 'SharedWithMe',
        component: { template: '<div />' },
        meta: { public: false },
      },
      {
        path: '/folder',
        name: 'FolderRootView',
        component: { template: '<div />' },
        meta: { public: false },
      },
      {
        path: '/activity',
        name: 'Activity',
        component: { template: '<div />' },
        meta: { public: false },
      },
      { path: '/login', name: 'Login', component: { template: '<div />' }, meta: { public: true } },
    ],
  });
  if (previous) {
    await router.push(previous);
  }
  await router.push('/files/file');
  // Memory history does not maintain the browser history's `back` state.
  if (previous) {
    router.options.history.state.back = previous;
  }
  const wrapper = mount(FileView, {
    props: { fileId: 'file' },
    global: {
      plugins: [OpenVue, router],
      stubs: { FilePreview: true, SharingDialog: true, FileComments: true },
    },
  });
  await flushPromises();
  return { wrapper, router };
}

describe('file page', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setActivePinia(createPinia());
    useAuthStore().user = { id: 'admin', role: 'admin' } as User;
    vi.spyOn(axios, 'get').mockResolvedValue({ data: [] });
    vi.spyOn(useFileManagerStore(), 'fetchFile').mockResolvedValue(file);
  });

  it('shows metadata, sharing, and an explicit download fallback for unsupported types', async () => {
    const download = vi.spyOn(useFileManagerStore(), 'downloadFile').mockResolvedValue();
    const { wrapper } = await render();
    expect(wrapper.get('h1').text()).toBe('archive.zip');
    expect(wrapper.get('h1').element.closest('[data-sentry-mask]')).not.toBeNull();
    expect(wrapper.get('.file-metadata').element.closest('[data-sentry-mask]')).not.toBeNull();
    expect(wrapper.get('.file-header-actions').element.closest('[data-sentry-mask]')).toBeNull();
    expect(wrapper.find('.file-folder').exists()).toBe(false);
    expect(wrapper.text()).not.toContain('Containing folder');
    expect(wrapper.text()).toContain('4.0 KB');
    expect(wrapper.text()).toContain('No preview available');
    expect(wrapper.findComponent(FilePreview).exists()).toBe(false);
    expect(download).not.toHaveBeenCalled();
    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Download')!
      .trigger('click');
    expect(download).toHaveBeenCalledWith(file);
    await wrapper.get('button[aria-haspopup="dialog"]').trigger('click');
    expect(wrapper.find('sharing-dialog-stub').exists()).toBe(true);
    wrapper.unmount();
  });

  it('loads a preview from fresh authorized metadata and returns direct links to their folder', async () => {
    vi.mocked(useFileManagerStore().fetchFile).mockResolvedValue({
      ...file,
      filename: 'report.pdf',
    });
    const { wrapper, router } = await render();
    expect(wrapper.findComponent(FilePreview).props('fileType')).toBe('application/pdf');
    expect(wrapper.findComponent(FilePreview).props('fileUrl')).toBe('/api/files/file/view');
    expect(wrapper.findComponent(FilePreview).props('downloadUrl')).toBe(
      '/api/files/file/download',
    );
    expect(wrapper.findComponent(FilePreview).props('maximized')).toBe(true);
    await wrapper.get('button[aria-label="Back to files"]').trigger('click');
    await flushPromises();
    expect(router.currentRoute.value.path).toBe('/folder/folder');
    wrapper.unmount();
  });

  it('does not offer sharing to link sessions or download to unreadable files', async () => {
    useAuthStore().user = { role: 'link' } as User;
    vi.mocked(useFileManagerStore().fetchFile).mockResolvedValue({
      ...file,
      capabilities: { ...file.capabilities!, can_read: false },
    });
    const { wrapper } = await render();
    expect(wrapper.find('button[aria-haspopup="dialog"]').exists()).toBe(false);
    expect(wrapper.text()).not.toContain('Download');
    expect(wrapper.findComponent(FilePreview).exists()).toBe(false);
    wrapper.unmount();
  });

  it('offers retry after an unavailable file without showing stale actions', async () => {
    vi.mocked(useFileManagerStore().fetchFile).mockRejectedValueOnce(new Error('gone'));
    const { wrapper } = await render();
    expect(wrapper.get('[role="alert"]').text()).toContain('File unavailable');
    expect(wrapper.find('button[aria-haspopup="dialog"]').exists()).toBe(false);
    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Try again')!
      .trigger('click');
    await flushPromises();
    expect(wrapper.get('h1').text()).toBe('archive.zip');
    wrapper.unmount();
  });

  it('opens comments on demand, preserves the panel on close, and lets Escape close it before navigating', async () => {
    const { wrapper, router } = await render();
    expect(wrapper.find('file-comments-stub').exists()).toBe(false);
    await wrapper.get('#file-comments-toggle').trigger('click');
    expect(wrapper.find('file-comments-stub').exists()).toBe(true);
    expect(wrapper.get('#file-comments-toggle').attributes('aria-expanded')).toBe('true');
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }));
    await flushPromises();
    expect(wrapper.get('#file-comments-toggle').attributes('aria-expanded')).toBe('false');
    expect(wrapper.find('file-comments-stub').exists()).toBe(true);
    expect(router.currentRoute.value.path).toBe('/files/file');
    wrapper.unmount();
  });

  it('never offers conversations to anonymous link sessions, even for readable files', async () => {
    useAuthStore().user = { role: 'link' } as User;
    const { wrapper } = await render();
    expect(wrapper.find('#file-comments-toggle').exists()).toBe(false);
    expect(wrapper.find('file-comments-stub').exists()).toBe(false);
    wrapper.unmount();
  });

  it('leaves Escape to comment action menus instead of closing the conversation underneath', async () => {
    const { wrapper, router } = await render();
    await wrapper.get('#file-comments-toggle').trigger('click');
    const popup = document.createElement('ul');
    popup.setAttribute('role', 'menu');
    document.body.append(popup);
    popup.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }),
    );
    await flushPromises();
    expect(wrapper.get('#file-comments-toggle').attributes('aria-expanded')).toBe('true');
    expect(router.currentRoute.value.path).toBe('/files/file');
    popup.remove();
    wrapper.unmount();
  });

  it('shows failed downloads on the page without poisoning the folder listing', async () => {
    const store = useFileManagerStore();
    vi.spyOn(store, 'downloadFile').mockImplementation(async () => {
      store.error = 'missing';
    });
    const { wrapper } = await render();
    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Download')!
      .trigger('click');
    await flushPromises();
    expect(wrapper.get('[role="alert"]').text()).toContain('couldn’t be downloaded');
    expect(store.error).toBe('');
    wrapper.unmount();
  });

  it('ignores an old metadata response after navigation to another file', async () => {
    let resolve!: (file: FileItem) => void;
    vi.mocked(useFileManagerStore().fetchFile).mockImplementationOnce(
      () =>
        new Promise((r) => {
          resolve = r;
        }),
    );
    const { wrapper } = await render();
    expect(wrapper.get('[role="status"]').text()).toContain('Loading file');
    await wrapper.setProps({ fileId: 'new-file' });
    await flushPromises();
    resolve({ ...file, filename: 'stale.zip' });
    await flushPromises();
    expect(wrapper.get('h1').text()).toBe('archive.zip');
    wrapper.unmount();
  });

  it.each(['/folder/another?page=3&sort_by=filename', '/shared-with-me?page=2', '/activity'])(
    'returns to the previous app page %s on Escape',
    async (previous) => {
      const { wrapper, router } = await render(previous);
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }));
      await flushPromises();
      expect(router.currentRoute.value.fullPath).toBe(previous);
      wrapper.unmount();
    },
  );

  it.each([undefined, '/login?redirect=/files/file'])(
    'returns fresh navigation with previous=%s to the parent, replacing the file entry',
    async (previous) => {
      const { wrapper, router } = await render(previous);
      const back = vi.spyOn(router, 'back');
      const replace = vi.spyOn(router, 'replace');
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }));
      await flushPromises();
      expect(router.currentRoute.value.path).toBe('/folder/folder');
      expect(back).not.toHaveBeenCalled();
      expect(replace).toHaveBeenCalledWith({ name: 'FolderView', params: { folderId: 'folder' } });
      wrapper.unmount();
    },
  );

  it('waits for file metadata before returning a fresh navigation to its parent', async () => {
    let resolve!: (file: FileItem) => void;
    vi.mocked(useFileManagerStore().fetchFile).mockImplementationOnce(
      () =>
        new Promise((r) => {
          resolve = r;
        }),
    );
    const { wrapper, router } = await render();
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }));
    await flushPromises();
    expect(router.currentRoute.value.path).toBe('/files/file');
    resolve(file);
    await flushPromises();
    expect(router.currentRoute.value.path).toBe('/folder/folder');
    wrapper.unmount();
  });

  it('leaves Escape to sharing and other dialogs instead of navigating underneath them', async () => {
    const { wrapper, router } = await render();
    await wrapper.get('button[aria-haspopup="dialog"]').trigger('click');
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }));
    await flushPromises();
    expect(router.currentRoute.value.path).toBe('/files/file');
    wrapper.findComponent({ name: 'SharingDialog' }).vm.$emit('close');
    await flushPromises();
    const dialog = document.createElement('div');
    dialog.setAttribute('role', 'dialog');
    document.body.append(dialog);
    try {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }));
      await flushPromises();
      expect(router.currentRoute.value.path).toBe('/files/file');
    } finally {
      dialog.remove();
      wrapper.unmount();
    }
  });

  it('ignores consumed/repeated Escape and removes its listener when unmounted', async () => {
    const { wrapper, router } = await render();
    const consumed = new KeyboardEvent('keydown', { key: 'Escape', cancelable: true });
    consumed.preventDefault();
    window.dispatchEvent(consumed);
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', repeat: true }));
    await flushPromises();
    expect(router.currentRoute.value.path).toBe('/files/file');
    wrapper.unmount();
    const replace = vi.spyOn(router, 'replace');
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }));
    await flushPromises();
    expect(replace).not.toHaveBeenCalled();
  });
});
