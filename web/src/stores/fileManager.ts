import axios from 'axios';
import { defineStore, storeToRefs } from 'pinia';

import type { FileItem, PaginatedResult, PaginationResponse } from '@/types/models';
import { defaultPagination } from '@/types/util';

import { useClientsStore } from './clients';

export type FileSortField = 'filename' | 'size' | 'uploaded_at';

export const useFileManagerStore = defineStore('fileManager', () => {
  interface CreateFolderOptions {
    targetFolderId?: string | null;
    reuseExisting?: boolean;
  }

  interface UploadFilesOptions {
    targetFolderId?: string | null;
    targetNames?: string[];
    refreshCurrentFolder?: boolean;
  }

  const clientsStore = useClientsStore();
  const { clientId } = storeToRefs(clientsStore);

  const files = ref<FileItem[]>([]);
  const folders = ref<FileItem[]>([]);

  /**
   * Folder hierarchy
   */
  const breadcrumbs = ref<FileItem[]>([]);

  const loading = ref(false);
  const error = ref<string | null>(null);

  /**
   * Current folder ID
   */
  const currentFolder = ref<string | null>(null);
  const pagination = ref<PaginationResponse>(defaultPagination());
  const sortField = ref<FileSortField>('uploaded_at');
  const sortDirection = ref<'asc' | 'desc'>('desc');

  // TODO: add permissions to API and use them here instead of assuming full access
  const canRead = ref(true);
  const canWrite = ref(true);

  async function changeFolder(folderId?: string) {
    currentFolder.value = folderId || null;
  }

  async function fetchFolders() {
    if (!currentFolder.value) {
      return;
    }
    try {
      const res = await axios.get<PaginatedResult<FileItem>>(
        `/api/folders/${currentFolder.value}?type=folder`,
      );
      folders.value = res.data.items || [];
    } catch (e: any) {
      console.error('Failed to fetch folders', e);
    }
  }

  async function listFoldersInFolder(
    folderId: string,
    page = 1,
    items: FileItem[] = [],
  ): Promise<FileItem[]> {
    const res = await axios.get<PaginatedResult<FileItem>>(`/api/folders/${folderId}`, {
      params: {
        type: 'folder',
        page,
        page_size: 100,
        sort_by: 'filename',
        sort_dir: 'asc',
      },
    });

    const nextItems = items.concat(res.data.items || []);
    const pageInfo = res.data.pagination || defaultPagination();
    if (!pageInfo.has_more || page >= pageInfo.total_pages) {
      return nextItems;
    }

    return listFoldersInFolder(folderId, page + 1, nextItems);
  }

  async function fetchFiles(page = 1, pageSize = 20) {
    if (!currentFolder.value) {
      return;
    }
    loading.value = true;
    error.value = null;
    try {
      void fetchFolders();
      const res = await axios.get<PaginatedResult<FileItem>>(
        `/api/folders/${currentFolder.value}`,
        {
          params: {
            page,
            page_size: pageSize,
            sort_by: sortField.value,
            sort_dir: sortDirection.value,
          },
        },
      );
      files.value = res.data.items || [];
      pagination.value = res.data.pagination || defaultPagination();
      if (files.value?.length) {
        clientId.value = files.value?.[0]?.client_id ?? null;
      }
    } catch (e: any) {
      error.value = e.response?.data?.error || 'Failed to fetch files';
    } finally {
      loading.value = false;
    }
  }

  /**
   * Fetch a single file by ID.
   *
   * If current folder not set, updates it to the parent folder of the file. This is used when
   * accessing a file directly via URL, where we may not have the folder context loaded.
   *
   * First checks if the file is already in the current list of files/folders, and if not, makes an
   * API call to fetch it. This is useful for the file view page, which may be accessed directly via
   * URL.
   *
   * @param fileId
   * @returns
   */
  async function fetchFile(fileId: string): Promise<FileItem> {
    if (currentFolder.value && files.value.length) {
      const existing = files.value.find((f) => f.id === fileId);
      if (existing) {
        currentFolder.value = existing.folder_id!;
        return existing;
      }
    }

    // If not found in current list, fetch from API
    try {
      const res = await axios.get<FileItem>(`/api/files/${fileId}`);
      const file = res.data;

      if (file.folder_id) {
        currentFolder.value = file.folder_id;
        clientId.value = file.client_id;
      }

      return file;
    } catch (e: any) {
      console.error('Failed to fetch file', e);
      throw new Error(e.response?.data?.error || 'Failed to fetch file');
    }
  }

  async function fetchBreadcrumbs() {
    if (!currentFolder.value) {
      breadcrumbs.value = [];
      return;
    }

    try {
      await axios.get<FileItem[]>(`/api/folders/${currentFolder.value}/breadcrumbs`).then((res) => {
        breadcrumbs.value = res.data || [];
      });
    } catch (e: any) {
      console.error('Failed to fetch breadcrumbs', e);
    }
  }

  async function createFolder(name: string, options: CreateFolderOptions = {}) {
    const targetFolderId = options.targetFolderId ?? currentFolder.value;
    if (!targetFolderId) {
      return null;
    }
    loading.value = true;
    error.value = null;
    try {
      if (options.reuseExisting) {
        const existingFolders = await listFoldersInFolder(targetFolderId);
        const existingFolder = existingFolders.find(
          (folder) => folder.filename.toLowerCase() === name.toLowerCase(),
        );
        if (existingFolder) {
          return existingFolder;
        }
      }

      const res = await axios.post<FileItem>(`/api/folders/${targetFolderId}`, { name });
      return res.data;
    } catch (e: any) {
      error.value = e.response?.data?.error || 'Failed to create folder';
    } finally {
      loading.value = false;
    }
    return null;
  }

  async function deleteItem(item: FileItem) {
    if (!item.id) {
      return;
    }
    loading.value = true;
    error.value = null;
    try {
      await axios.delete(`/api/files/${item.id}`);
      await fetchFiles(pagination.value.page, pagination.value.page_size);
    } catch (e: any) {
      error.value = e.response?.data?.error || 'Failed to delete item';
    } finally {
      loading.value = false;
    }
  }

  async function uploadFiles(
    filesToUpload: File[],
    notifyClient = true,
    onProgress?: (percent: number) => void,
    options: UploadFilesOptions = {},
  ) {
    const targetFolderId = options.targetFolderId ?? currentFolder.value;
    if (!targetFolderId) {
      error.value = 'No folder selected';
      return { ok: false as const, error: 'No folder selected' };
    }

    loading.value = true;
    error.value = null;
    try {
      const formData = new FormData();
      filesToUpload.forEach((f, index) => {
        const name = options.targetNames?.[index] ?? f.name;
        formData.append('files', f, name);
      });
      formData.append('notify', notifyClient ? 'true' : 'false');
      await axios.post(`/api/folders/${targetFolderId}/upload`, formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
        onUploadProgress: (e) => {
          if (onProgress && e.total) {
            onProgress(e.progress ? e.progress * 100 : Math.round((e.loaded * 100) / e.total));
          }
        },
      });
      if (options.refreshCurrentFolder ?? true) {
        await fetchFiles(pagination.value.page, pagination.value.page_size);
      }
      return { ok: true as const };
    } catch (e: any) {
      const response = e.response?.data;
      const message = response?.error || 'Failed to upload files';
      error.value = message;
      if (response?.code) {
        return { ok: false as const, error: message, code: response.code, name: response.name };
      }
      return { ok: false as const, error: message };
    } finally {
      loading.value = false;
    }
  }

  async function checkExistingFilenames(names: string[], targetFolderId = currentFolder.value) {
    if (!targetFolderId || names.length === 0) {
      return [] as string[];
    }
    const res = await axios.post<{ existing: string[] }>(`/api/folders/${targetFolderId}/exists`, {
      names,
    });
    return res.data.existing || [];
  }

  async function downloadFile(item: FileItem) {
    try {
      const res = await axios.get(`/api/files/${item.id}/download`, {
        responseType: 'blob',
      });
      const url = window.URL.createObjectURL(res.data);
      const a = document.createElement('a');
      a.href = url;
      a.download = item.filename;
      a.click();
      window.URL.revokeObjectURL(url);
    } catch (e: any) {
      error.value = e.response?.data?.error || 'Failed to download file';
    }
  }

  async function downloadZip(item: FileItem) {
    try {
      const res = await axios.get(`/api/folders/${item.id}/download-zip`, {
        responseType: 'blob',
      });
      const url = window.URL.createObjectURL(res.data);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${item.filename}.zip`;
      a.click();
      window.URL.revokeObjectURL(url);
    } catch (e: any) {
      error.value = e.response?.data?.error || 'Failed to download zip';
    }
  }

  watch(
    currentFolder,
    async () => {
      await Promise.all([fetchFiles(1, pagination.value.page_size), fetchBreadcrumbs()]);
      if (!clientId.value && breadcrumbs.value.length) {
        clientId.value = breadcrumbs.value[0].client_id;
      }
    },
    {
      immediate: true,
    },
  );

  return {
    folders,
    files,
    breadcrumbs,
    loading,
    error,
    currentFolder,
    clientId,
    pagination,
    sortField,
    sortDirection,
    canRead,
    canWrite,

    changeFolder,
    fetchFiles,
    fetchFile,
    createFolder,
    deleteItem,
    uploadFiles,
    checkExistingFilenames,
    downloadFile,
    downloadZip,
  };
});
