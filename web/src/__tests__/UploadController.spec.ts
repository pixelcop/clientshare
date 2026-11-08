import { flushPromises, mount } from '@vue/test-utils';
import axios from 'axios';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent } from 'vue';

import { useUploadController } from '@/composables/useUploadController';
import { useAuthStore } from '@/stores/auth';
import { useFileManagerStore } from '@/stores/fileManager';
import type { User } from '@/types/models';
import type { WorkspaceContext } from '@/types/workspace';

describe('upload controller', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setActivePinia(createPinia());
    useAuthStore().user = { id: 'user', role: 'admin' } as User;
    vi.spyOn(axios, 'get').mockResolvedValue({ data: [] });
  });

  function setup() {
    const store = useFileManagerStore();
    store.currentFolder = 'original';
    store.context = {
      capabilities: { can_read: true, can_upload: true, can_delete: true, can_share: true },
    } as WorkspaceContext;
    let controller!: ReturnType<typeof useUploadController>;
    const wrapper = mount(
      defineComponent({
        setup() {
          controller = useUploadController({ canNotify: () => true, notifyClient: () => false });
          return () => null;
        },
      }),
    );
    return { store, controller, wrapper };
  }

  it('captures the destination before duplicate checks and keeps it through rename', async () => {
    const { store, controller, wrapper } = setup();
    let finishCheck!: (names: string[]) => void;
    const check = vi.spyOn(store, 'checkExistingFilenames').mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishCheck = resolve;
        }),
    );
    const upload = vi.spyOn(store, 'uploadFiles').mockResolvedValue({ ok: true });
    vi.spyOn(store, 'fetchFiles').mockResolvedValue();
    const input = { files: [new File(['test'], 'report.txt')], value: '' };
    const pending = controller.onFileChange({ target: input } as unknown as Event);
    store.currentFolder = 'other';
    finishCheck(['report.txt']);
    await pending;
    expect(controller.showRename.value).toBe(true);
    controller.renameItems.value[0].name = 'renamed.txt';
    check.mockResolvedValue([]);
    await controller.submitRename();
    expect(check).toHaveBeenLastCalledWith(['renamed.txt'], 'original');
    expect(upload).toHaveBeenCalledWith(
      expect.any(Array),
      false,
      expect.any(Function),
      expect.objectContaining({ targetFolderId: 'original', targetNames: ['renamed.txt'] }),
    );
    wrapper.unmount();
  });

  it('preserves directory structure, ignores junk and honors notification preference', async () => {
    const { store, controller, wrapper } = setup();
    vi.spyOn(store, 'checkExistingFilenames').mockResolvedValue([]);
    const create = vi
      .spyOn(store, 'createFolder')
      .mockImplementation(async (name) => ({ id: name, filename: name }) as never);
    const upload = vi.spyOn(store, 'uploadFiles').mockResolvedValue({ ok: true });
    vi.spyOn(store, 'fetchFiles').mockResolvedValue();
    const file = new File(['contents'], 'report.txt');
    Object.defineProperty(file, 'webkitRelativePath', { value: 'Documents/2026/report.txt' });
    const junk = new File(['junk'], '.DS_Store');
    Object.defineProperty(junk, 'webkitRelativePath', { value: 'Documents/.DS_Store' });
    await controller.onDirectoryChange({
      target: { files: [file, junk], value: '' },
    } as unknown as Event);
    expect(create).toHaveBeenCalledWith(
      'Documents',
      expect.objectContaining({ targetFolderId: 'original' }),
    );
    expect(create).toHaveBeenCalledWith(
      '2026',
      expect.objectContaining({ targetFolderId: 'Documents' }),
    );
    expect(upload).toHaveBeenCalledWith(
      [file],
      false,
      expect.any(Function),
      expect.objectContaining({ targetFolderId: '2026' }),
    );
    wrapper.unmount();
  });

  it('ignores internal/text drags and clears a valid file drag on Escape', async () => {
    const { controller, wrapper } = setup();
    const preventDefault = vi.fn();
    controller.onDragOver({
      dataTransfer: { types: ['text/plain'] },
      preventDefault,
    } as unknown as DragEvent);
    expect(preventDefault).not.toHaveBeenCalled();
    expect(controller.dragging.value).toBe(false);
    controller.onDragOver({
      dataTransfer: { types: ['Files'] },
      preventDefault,
    } as unknown as DragEvent);
    expect(controller.dragging.value).toBe(true);
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await flushPromises();
    expect(controller.dragging.value).toBe(false);
    wrapper.unmount();
  });

  it('keeps empty dropped folders without requiring a file upload', async () => {
    const { store, controller, wrapper } = setup();
    const create = vi.spyOn(store, 'createFolder').mockResolvedValue({ id: 'empty' } as never);
    const upload = vi.spyOn(store, 'uploadFiles');
    vi.spyOn(store, 'fetchFiles').mockResolvedValue();
    const directory = {
      name: 'Empty',
      isDirectory: true,
      isFile: false,
      createReader: () => ({ readEntries: (done: (entries: unknown[]) => void) => done([]) }),
    };
    await controller.onDrop({
      preventDefault: vi.fn(),
      dataTransfer: {
        types: ['Files'],
        files: [],
        items: [{ kind: 'file', webkitGetAsEntry: () => directory }],
      },
    } as unknown as DragEvent);
    expect(create).toHaveBeenCalledWith(
      'Empty',
      expect.objectContaining({ targetFolderId: 'original' }),
    );
    expect(upload).not.toHaveBeenCalled();
    expect(controller.uploadProgress.value).toBe(100);
    wrapper.unmount();
  });

  it('stops remaining folder batches and preserves partial-completion feedback', async () => {
    const { store, controller, wrapper } = setup();
    vi.spyOn(store, 'checkExistingFilenames').mockResolvedValue([]);
    vi.spyOn(store, 'createFolder').mockImplementation(async (name) => ({ id: name }) as never);
    vi.spyOn(store, 'fetchFiles').mockResolvedValue();
    const upload = vi.spyOn(store, 'uploadFiles').mockImplementation(async () => {
      controller.stopUpload();
      return { ok: true };
    });
    const files = ['First', 'Second'].map((name) => {
      const file = new File(['test'], `${name}.txt`);
      Object.defineProperty(file, 'webkitRelativePath', { value: `Documents/${name}/${name}.txt` });
      return file;
    });
    await controller.onDirectoryChange({ target: { files, value: '' } } as unknown as Event);
    expect(upload).toHaveBeenCalledTimes(1);
    expect(controller.uploadError.value).toContain('Completed files and folders were kept');
    expect(controller.stopped.value).toBe(true);
    wrapper.unmount();
  });
});
