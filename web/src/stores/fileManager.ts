import axios from 'axios';
import { defineStore, storeToRefs } from 'pinia';

import type {
  AccessCapabilities,
  FileItem,
  PaginatedResult,
  PaginationResponse,
} from '@/types/models';
import { defaultPagination } from '@/types/util';
import type { WorkspaceContext, WorkspaceListing } from '@/types/workspace';

import { useAuthStore } from './auth';
import { useClientsStore } from './clients';

export type FileSortField = 'filename' | 'size' | 'uploaded_at';
export interface FileListingOptions {
  page: number;
  pageSize: number;
  sortField: FileSortField;
  sortDirection: 'asc' | 'desc';
}

export const useFileManagerStore = defineStore('fileManager', () => {
  interface CreateFolderOptions {
    destinationCapabilities?: AccessCapabilities;
    targetFolderId?: string | null;
    reuseExisting?: boolean;
    signal?: AbortSignal;
  }

  interface UploadFilesOptions {
    destinationCapabilities?: AccessCapabilities;
    targetFolderId?: string | null;
    targetNames?: string[];
    refreshCurrentFolder?: boolean;
    signal?: AbortSignal;
  }

  const clientsStore = useClientsStore();
  const { clientId } = storeToRefs(clientsStore);

  const items = ref<FileItem[]>([]);
  const files = computed(() => items.value.filter((item) => item.type !== 'folder'));
  const folders = computed(() => items.value.filter((item) => item.type === 'folder'));

  /**
   * Folder hierarchy
   */
  const breadcrumbs = ref<FileItem[]>([]);

  const loading = ref(false);
  const error = ref<string | null>(null);
  const context = ref<WorkspaceContext | null>(null);
  const loaded = ref(false);
  const isEmpty = computed(
    () => loaded.value && !loading.value && !error.value && items.value.length === 0,
  );
  let requestVersion = 0;

  /**
   * Current folder ID
   */
  const currentFolder = ref<string | null>(null);
  const pagination = ref<PaginationResponse>(defaultPagination());
  const sortField = ref<FileSortField>('uploaded_at');
  const sortDirection = ref<'asc' | 'desc'>('desc');
  const requestedPage = ref(1);
  const requestedPageSize = ref(20);

  const auth = useAuthStore();
  const noCapabilities: AccessCapabilities = {
    can_read: false,
    can_upload: false,
    can_delete: false,
    can_share: false,
  };
  const capabilities = computed(() => context.value?.capabilities ?? noCapabilities);
  const canRead = computed(() => !!auth.user && capabilities.value.can_read);
  const canWrite = computed(() => !!auth.user && capabilities.value.can_upload);

  async function changeFolder(folderId?: string) {
    currentFolder.value = folderId || null;
  }

  async function listFoldersInFolder(
    folderId: string,
    page = 1,
    collectedFolders: FileItem[] = [],
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

    const nextItems = collectedFolders.concat(res.data.items || []);
    const pageInfo = res.data.pagination || defaultPagination();
    if (!pageInfo.has_more || page >= pageInfo.total_pages) {
      return nextItems;
    }

    return listFoldersInFolder(folderId, page + 1, nextItems);
  }

  async function fetchFiles(page = requestedPage.value, pageSize = requestedPageSize.value) {
    const folderId = currentFolder.value;
    if (!folderId) {
      return;
    }
    requestVersion += 1;
    const version = requestVersion;
    loading.value = true;
    error.value = null;
    loaded.value = false;
    try {
      const [res, crumbs] = await Promise.all([
        axios.get<WorkspaceListing>(`/api/folders/${folderId}`, {
          params: {
            type: 'all',
            page,
            page_size: pageSize,
            sort_by: sortField.value,
            sort_dir: sortDirection.value,
          },
        }),
        axios.get<FileItem[]>(`/api/folders/${folderId}/breadcrumbs`),
      ]);
      if (version !== requestVersion || folderId !== currentFolder.value) {
        return;
      }
      items.value = res.data.items || [];
      breadcrumbs.value = crumbs.data || [];
      pagination.value = res.data.pagination || defaultPagination();
      requestedPage.value = pagination.value.page;
      requestedPageSize.value = pagination.value.page_size;
      context.value = res.data.context;
      clientId.value = res.data.context?.client_id ?? null;
      loaded.value = true;
    } catch (e: any) {
      if (version !== requestVersion || folderId !== currentFolder.value) {
        return;
      }
      error.value = e.response?.data?.error || 'Failed to fetch files';
      items.value = [];
      breadcrumbs.value = [];
      context.value = null;
      clientId.value = null;
    } finally {
      if (version === requestVersion) {
        loading.value = false;
      }
    }
  }

  function configureListing(options: FileListingOptions, refresh = true) {
    const changed =
      requestedPage.value !== options.page ||
      requestedPageSize.value !== options.pageSize ||
      sortField.value !== options.sortField ||
      sortDirection.value !== options.sortDirection;
    requestedPage.value = options.page;
    requestedPageSize.value = options.pageSize;
    sortField.value = options.sortField;
    sortDirection.value = options.sortDirection;
    if (changed && refresh) {
      void fetchFiles(options.page, options.pageSize);
    }
  }

  /** Fetching one file does not establish authority to list its parent. */
  async function fetchFile(fileId: string): Promise<FileItem> {
    // Cached rows are not an authorization snapshot across expiry/revocation.
    try {
      const res = await axios.get<FileItem>(`/api/files/${fileId}`);
      const file = res.data;

      return file;
    } catch (e: any) {
      console.error('Failed to fetch file', e);
      throw new Error(e.response?.data?.error || 'Failed to fetch file', { cause: e });
    }
  }

  async function createFolder(name: string, options: CreateFolderOptions = {}) {
    if (!auth.user || !(options.destinationCapabilities?.can_upload ?? canWrite.value)) {
      error.value = 'This link does not allow uploads or changes';
      return null;
    }
    const targetFolderId = options.targetFolderId ?? currentFolder.value;
    if (!targetFolderId) {
      return null;
    }
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

      const res = await axios.post<FileItem>(
        `/api/folders/${targetFolderId}`,
        { name },
        { signal: options.signal },
      );
      return res.data;
    } catch (e: any) {
      error.value = e.response?.data?.error || 'Failed to create folder';
    }
    return null;
  }

  async function deleteItem(item: FileItem) {
    if (!auth.user || !item.capabilities?.can_delete) {
      error.value = 'You do not have permission to delete this item';
      return;
    }
    if (!item.id) {
      return;
    }
    error.value = null;
    try {
      await axios.delete(`/api/files/${item.id}`);
      await fetchFiles(pagination.value.page, pagination.value.page_size);
    } catch (e: any) {
      error.value = e.response?.data?.error || 'Failed to delete item';
    }
  }

  async function uploadFiles(
    filesToUpload: File[],
    notifyClient = true,
    onProgress?: (percent: number) => void,
    options: UploadFilesOptions = {},
  ) {
    if (!auth.user || !(options.destinationCapabilities?.can_upload ?? canWrite.value)) {
      error.value = 'This link does not allow uploads or changes';
      return { ok: false as const, error: error.value };
    }
    const targetFolderId = options.targetFolderId ?? currentFolder.value;
    if (!targetFolderId) {
      error.value = 'No folder selected';
      return { ok: false as const, error: 'No folder selected' };
    }

    try {
      const formData = new FormData();
      filesToUpload.forEach((f, index) => {
        const name = options.targetNames?.[index] ?? f.name;
        formData.append('files', f, name);
      });
      formData.append('notify', notifyClient ? 'true' : 'false');
      await axios.post(`/api/folders/${targetFolderId}/upload`, formData, {
        signal: options.signal,
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
      const message = axios.isCancel(e)
        ? 'Upload stopped. Completed files and folders were kept.'
        : response?.error || 'Failed to upload files';
      if (response?.code) {
        return { ok: false as const, error: message, code: response.code, name: response.name };
      }
      return { ok: false as const, error: message };
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
      requestVersion += 1;
      items.value = [];
      breadcrumbs.value = [];
      context.value = null;
      clientId.value = null;
      loaded.value = false;
      loading.value = false;
      error.value = null;
      pagination.value = { ...defaultPagination(), page_size: requestedPageSize.value };
      await fetchFiles();
    },
    {
      immediate: true,
      flush: 'sync',
    },
  );

  watch(
    // Watch identity values separately. A getter returning a new array also
    // fires when auth/me replaces the user object with the same identity.
    [
      () => auth.user?.tenant_id,
      () => auth.user?.id,
      () => auth.user?.role,
      () => auth.user?.session_id,
      () => auth.user?.link_id,
    ],
    () => {
      requestVersion += 1;
      currentFolder.value = null;
      items.value = [];
      breadcrumbs.value = [];
      context.value = null;
      clientId.value = null;
      loaded.value = false;
      error.value = null;
      loading.value = false;
      requestedPage.value = 1;
      requestedPageSize.value = 20;
    },
    { flush: 'sync' },
  );

  return {
    items,
    folders,
    files,
    breadcrumbs,
    loading,
    error,
    context,
    loaded,
    isEmpty,
    currentFolder,
    clientId,
    pagination,
    sortField,
    sortDirection,
    canRead,
    canWrite,
    capabilities,

    changeFolder,
    configureListing,
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
