<script setup lang="ts">
import { storeToRefs } from 'pinia';
import type { PageState } from 'primevue';

import { useAuthStore } from '@/stores/auth';
import type { FileSortField } from '@/stores/fileManager';
import { useFileManagerStore } from '@/stores/fileManager';
import type { FileItem } from '@/types/models';
import { getFileType, getFileURL } from '@/types/util';

import Breadcrumb from './Breadcrumb.vue';
import FileListItem from './FileListItem.vue';
import FilePreviewModal from './FilePreviewModal.vue';
import FolderList from './FolderList.vue';
import NewFolderModal from './NewFolderModal.vue';

const router = useRouter();
const fileManager = useFileManagerStore();
const { files, folders, breadcrumbs, pagination, sortField, sortDirection } =
  storeToRefs(fileManager);
const { user } = storeToRefs(useAuthStore());

const showNewFolder = ref(false);
const showHeader = computed(() => ['client', 'customer'].includes(user.value?.role ?? ''));
const showPreview = ref(false);
const previewItem = ref<FileItem | null>(null);
const pageSizeOptions = [10, 20, 50];
const first = computed(() => Math.max(0, (pagination.value.page - 1) * pagination.value.page_size));

const defaultDirectionByField: Record<FileSortField, 'asc' | 'desc'> = {
  filename: 'asc',
  size: 'asc',
  uploaded_at: 'desc',
};

function sortIconClass(field: FileSortField) {
  if (sortField.value !== field) {
    return 'pi pi-sort-alt text-xs text-slate-400';
  }
  return sortDirection.value === 'desc' ? 'pi pi-arrow-down' : 'pi pi-arrow-up';
}

function sortBy(field: FileSortField) {
  if (sortField.value === field) {
    sortDirection.value = sortDirection.value === 'desc' ? 'asc' : 'desc';
  } else {
    sortField.value = field;
    sortDirection.value = defaultDirectionByField[field];
  }
  fileManager.fetchFiles(1, pagination.value.page_size);
}

const handlePageChange = (event: PageState) => {
  fileManager.fetchFiles(event.page + 1, event.rows);
};

function enterFolder(item: FileItem) {
  if (item.type === 'folder') {
    router.push(`/folder/${item.id}`);
  }
}

function handleDownload() {
  if (previewItem.value) {
    fileManager.downloadFile(previewItem.value);
  }
}

function previewFile(item: FileItem) {
  previewItem.value = item;
  showPreview.value = true;
}

function closePreview() {
  showPreview.value = false;
  previewItem.value = null;
}
</script>

<template>
  <Card class="mt-4 shadow border border-slate-200">
    <template v-if="showHeader" #header>
      <div class="px-4 pt-3 border-b border-slate-200 flex items-center justify-between">
        <h2 class="font-semibold text-slate-900">Files</h2>
      </div>
    </template>
    <template #content>
      <div class="space-y-6">
        <div class="flex items-center justify-between mb-4">
          <Breadcrumb v-if="breadcrumbs.length > 1" />
          <div v-else>&nbsp;</div>
          <div class="flex items-center gap-2">
            <Button
              @click="showNewFolder = true"
              icon="pi pi-plus"
              label="New Folder"
              severity="info"
              size="small"
            />
          </div>
        </div>

        <FolderList v-if="folders.length > 0" :items="folders" @open="enterFolder" />
        <div class="space-y-2 overflow-x-auto">
          <div class="flex items-center justify-between">
            <h3 class="text-sm font-semibold text-slate-700">Files</h3>
            <span v-if="files.length === 0" class="text-xs text-slate-400">No files</span>
          </div>
          <table v-if="files.length > 0" class="min-w-full">
            <thead>
              <tr>
                <th
                  class="px-0 pr-4 py-2 sm:px-4 sm:py-2 text-left w-full"
                  :aria-sort="
                    sortField === 'filename'
                      ? sortDirection === 'desc'
                        ? 'descending'
                        : 'ascending'
                      : 'none'
                  "
                >
                  <button
                    type="button"
                    class="inline-flex items-center gap-2 text-left font-medium text-slate-700 hover:text-slate-900 cursor-pointer"
                    @click="sortBy('filename')"
                  >
                    <span>Name</span>
                    <i :class="sortIconClass('filename')" aria-hidden="true"></i>
                  </button>
                </th>
                <!-- <th class="px-4 py-2 text-left hidden sm:table-cell whitespace-nowrap w-[1%]">Type</th> -->
                <th
                  class="px-4 py-2 text-left hidden sm:table-cell whitespace-nowrap w-[1%]"
                  :aria-sort="
                    sortField === 'size'
                      ? sortDirection === 'desc'
                        ? 'descending'
                        : 'ascending'
                      : 'none'
                  "
                >
                  <button
                    type="button"
                    class="inline-flex items-center gap-2 text-left font-medium text-slate-700 hover:text-slate-900 cursor-pointer"
                    @click="sortBy('size')"
                  >
                    <span>Size</span>
                    <i :class="sortIconClass('size')" aria-hidden="true"></i>
                  </button>
                </th>
                <th
                  class="px-4 py-2 text-left hidden sm:table-cell whitespace-nowrap w-[1%]"
                  :aria-sort="
                    sortField === 'uploaded_at'
                      ? sortDirection === 'desc'
                        ? 'descending'
                        : 'ascending'
                      : 'none'
                  "
                >
                  <button
                    type="button"
                    class="inline-flex items-center gap-2 text-left font-medium text-slate-700 hover:text-slate-900 cursor-pointer"
                    @click="sortBy('uploaded_at')"
                  >
                    <span>Added</span>
                    <i :class="sortIconClass('uploaded_at')" aria-hidden="true"></i>
                  </button>
                </th>
                <th class="px-0 sm:px-4 py-2 w-[1%]"></th>
              </tr>
            </thead>
            <tbody>
              <FileListItem v-for="item in files" :key="item.id" :item @open="previewFile(item)" />
            </tbody>
          </table>
          <div v-if="!fileManager.loading && files.length === 0" class="text-sm text-slate-500">
            No files available.
          </div>
        </div>
      </div>
    </template>
    <template v-if="pagination.total_items > 0" #footer>
      <div class="flex items-center justify-between pt-3 border-t border-slate-200">
        <div class="text-sm text-slate-600">
          Showing {{ (pagination.page - 1) * pagination.page_size + 1 }} to
          {{ Math.min(pagination.page * pagination.page_size, pagination.total_items) }} of
          {{ pagination.total_items }}
        </div>
        <Paginator
          :alwaysShow="false"
          :first="first"
          :rows="pagination.page_size"
          :totalRecords="pagination.total_items"
          :rowsPerPageOptions="pageSizeOptions"
          @page="handlePageChange"
        />
      </div>
    </template>
  </Card>

  <!-- Preview Modal (simple text/image/pdf) -->
  <FilePreviewModal
    v-if="showPreview && previewItem"
    :visible="showPreview"
    :file="previewItem"
    :fileUrl="getFileURL(previewItem, true)"
    :fileName="previewItem.filename"
    :fileType="getFileType(previewItem)"
    @close="closePreview"
    @download="handleDownload"
  />

  <NewFolderModal v-if="showNewFolder" :show="showNewFolder" @close="showNewFolder = false" />
</template>
