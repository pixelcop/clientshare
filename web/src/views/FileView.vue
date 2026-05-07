<script setup lang="ts">
import { storeToRefs } from 'pinia';

import FilePreview from '@/components/FilePreview.vue';
import { useClientsStore } from '@/stores/clients';
import { useFileManagerStore } from '@/stores/fileManager';
import type { FileItem } from '@/types/models';
import { getFileType, getFileURL } from '@/types/util';

const { fileId } = defineProps<{
  fileId: string;
}>();

const { currentClient } = storeToRefs(useClientsStore());
const fileManager = useFileManagerStore();
const file = ref<FileItem | null>(null);

const downloadUrl = computed(() => {
  if (!file.value) {
    return '';
  }
  return getFileURL(file.value, true);
});

const homeRoute = computed(() => {
  if (file.value) {
    return { name: 'FolderView', params: { folderId: file.value.folder_id } };
  }
  return { name: 'Clients' };
});

onMounted(async () => {
  if (fileId) {
    file.value = await fileManager.fetchFile(fileId);
  }
});
</script>

<template>
  <h1 class="text-2xl font-bold">{{ currentClient?.name || 'File Manager' }}</h1>
  <Card class="mt-4 shadow border border-slate-200 h-full">
    <template #header>
      <div class="px-4 pt-3 border-b border-slate-200 flex items-center justify-between gap-3">
        <h2 class="font-semibold text-slate-900 truncate" :title="file?.filename || 'File preview'">
          {{ file?.filename || 'File preview' }}
        </h2>
        <div class="flex items-center gap-2">
          <Button
            as="router-link"
            :to="homeRoute"
            icon="pi pi-arrow-left"
            label="Back"
            severity="secondary"
            outlined
            size="small"
          />
          <Button
            as="a"
            :href="downloadUrl"
            :download="file?.filename"
            icon="pi pi-download"
            label="Download"
            severity="info"
            size="small"
          />
        </div>
      </div>
    </template>
    <template #content>
      <Breadcrumb :file />
      <FilePreview
        v-if="file"
        :file-type="getFileType(file)"
        :file-url="downloadUrl"
        maximized
        class="pt-2"
      />
    </template>
  </Card>
</template>

<style scoped>
:deep(.p-card-body),
:deep(.p-card-content) {
  height: 100%;
}
</style>
