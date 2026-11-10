<script setup lang="ts">
import '@/assets/file-rows.css';

import type { PageState } from 'openvue';
import { storeToRefs } from 'pinia';

import { useWorkspacePresentation } from '@/composables/useWorkspacePresentation';
import type { FileSortField } from '@/stores/fileManager';
import { useFileManagerStore } from '@/stores/fileManager';
import type { FileItem } from '@/types/models';
import { canPreviewFile, getFileType, getFileURL } from '@/types/util';
import { fileListingQuery, filePageSizes, parseFileListingQuery } from '@/utils/fileListingQuery';

import Dropzone from './Dropzone.vue';
import FileListItem from './FileListItem.vue';
import FilePreviewModal from './FilePreviewModal.vue';

withDefaults(defineProps<{ canNotify?: boolean }>(), { canNotify: true });
const emit = defineEmits<{ uploaded: [] }>();

const router = useRouter();
const route = useRoute();
const fileManager = useFileManagerStore();
const { sortField, sortDirection } = storeToRefs(fileManager);
const presentation = useWorkspacePresentation();
const items = computed(() => presentation.value.items);
const pagination = computed(() => presentation.value.pagination);
const canWrite = computed(() => presentation.value.canWrite);
const canRead = computed(() => presentation.value.canRead);
const context = computed(() => presentation.value.context);
const isEmpty = computed(() => presentation.value.isEmpty);
const showPreview = ref(false);
const previewItem = ref<FileItem | null>(null);
const pageSizeOptions = filePageSizes;
const first = computed(() => Math.max(0, (pagination.value.page - 1) * pagination.value.page_size));
const emptyHeading = computed(() => {
  if (!canRead.value && canWrite.value) {
    return `Upload documents to ${context.value?.client_name || 'this workspace'}`;
  }
  if (!canWrite.value) {
    return 'No files shared here yet';
  }
  return context.value?.folder.id === context.value?.root_folder_id
    ? 'No files yet'
    : 'This folder is empty';
});

const defaultDirectionByField: Record<FileSortField, 'asc' | 'desc'> = {
  filename: 'asc',
  size: 'asc',
  uploaded_at: 'desc',
};

function sortIconClass(field: FileSortField) {
  if (sortField.value !== field) {
    return 'oi oi-sort-alt text-xs text-slate-400';
  }
  return sortDirection.value === 'desc' ? 'oi oi-arrow-down text-xs' : 'oi oi-arrow-up text-xs';
}

function sortBy(field: FileSortField) {
  void router.push({
    query: fileListingQuery(
      {
        ...parseFileListingQuery(route.query),
        page: 1,
        sortField: field,
        sortDirection:
          sortField.value === field
            ? sortDirection.value === 'desc'
              ? 'asc'
              : 'desc'
            : defaultDirectionByField[field],
      },
      route.query,
    ),
  });
}

const handlePageChange = (event: PageState) => {
  void router.push({
    query: fileListingQuery(
      {
        ...parseFileListingQuery(route.query),
        page: event.rows === pagination.value.page_size ? event.page + 1 : 1,
        pageSize: event.rows,
      },
      route.query,
    ),
  });
};

function handleDownload() {
  if (previewItem.value) {
    fileManager.downloadFile(previewItem.value);
  }
}

function openItem(item: FileItem) {
  if (!fileManager.canRead) {
    return;
  }
  if (item.type === 'folder') {
    void router.push(`/folder/${item.id}`);
    return;
  }
  if (!canPreviewFile(item)) {
    void fileManager.downloadFile(item);
    return;
  }
  previewItem.value = item;
  showPreview.value = true;
}

function closePreview() {
  showPreview.value = false;
  previewItem.value = null;
}
watch(() => fileManager.currentFolder, closePreview);
</script>

<template>
  <Dropzone :canNotify @uploaded="emit('uploaded')">
    <template #context>
      <h2 id="collect-files" tabindex="-1" class="text-base font-semibold">Files</h2>
    </template>
    <template #actions><slot name="actions" /></template>
    <p v-if="fileManager.loading && !presentation.loaded" role="status" class="py-12 text-center">
      Loading files…
    </p>
    <div v-else-if="fileManager.error" role="alert" class="py-8 space-y-3">
      <p>{{ fileManager.error }}</p>
      <Button label="Try again" severity="secondary" @click="fileManager.fetchFiles()" />
    </div>
    <div
      v-else-if="isEmpty || (presentation.loaded && !canRead && canWrite)"
      class="workspace-empty"
    >
      <i class="oi oi-folder" aria-hidden="true" />
      <h3>{{ emptyHeading }}</h3>
      <p v-if="canWrite">Use Upload or drag files here to get started.</p>
      <p v-else>Files will appear here when they’re added.</p>
      <slot v-if="canRead && isEmpty" name="empty" />
    </div>
    <template v-else-if="presentation.loaded">
      <div class="space-y-6">
        <div v-if="items.length" class="overflow-x-auto">
          <table class="file-table" aria-label="Files and folders">
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
                  class="file-size hidden sm:table-cell"
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
                  class="file-added hidden sm:table-cell"
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
              <FileListItem v-for="item in items" :key="item.id" :item @open="openItem(item)" />
            </tbody>
          </table>
        </div>
      </div>
    </template>
    <template v-if="!fileManager.error && pagination.total_items > 0">
      <div
        class="flex flex-col gap-3 sm:flex-row items-center justify-between pt-3 border-t border-slate-200"
      >
        <div class="text-sm text-slate-600 whitespace-nowrap">
          Showing {{ (pagination.page - 1) * pagination.page_size + 1 }} to
          {{ Math.min(pagination.page * pagination.page_size, pagination.total_items) }} of
          {{ pagination.total_items }}
        </div>
        <Paginator
          :template="{
            '640px': 'PrevPageLink CurrentPageReport NextPageLink RowsPerPageDropdown',
            default:
              'FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink RowsPerPageDropdown',
          }"
          currentPageReportTemplate="Page {currentPage} of {totalPages}"
          aria-label="File list pages"
          :alwaysShow="false"
          :first="first"
          :rows="pagination.page_size"
          :totalRecords="pagination.total_items"
          :rowsPerPageOptions="pageSizeOptions"
          @page="handlePageChange"
        />
      </div>
    </template>
  </Dropzone>

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
</template>

<style scoped>
.workspace-empty {
  padding: 48px 16px;
  text-align: center;
}
.workspace-empty > .oi {
  font-size: 48px;
  color: var(--p-text-muted-color);
}
.workspace-empty h3 {
  margin: 16px 0 8px;
  font-size: 22px;
  font-weight: 600;
}
.workspace-empty p {
  font-size: 14px;
  color: var(--p-text-muted-color);
}
</style>
