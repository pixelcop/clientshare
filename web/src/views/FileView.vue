<script setup lang="ts">
import FilePreview from '@/components/FilePreview.vue';
import SharingDialog from '@/components/SharingDialog.vue';
import { useAuthStore } from '@/stores/auth';
import { useFileManagerStore } from '@/stores/fileManager';
import type { FileItem } from '@/types/models';
import { getFileType, getFileURL } from '@/types/util';

const { fileId } = defineProps<{
  fileId: string;
}>();

const auth = useAuthStore();
const fileManager = useFileManagerStore();
const file = ref<FileItem | null>(null);
const loading = ref(false);
const showShare = ref(false);
const error = ref('');
let requestVersion = 0;

const downloadUrl = computed(() => {
  if (!file.value) {
    return '';
  }
  return getFileURL(file.value, true);
});

const homeRoute = computed(() => {
  if (file.value?.folder_id) {
    return { name: 'FolderView', params: { folderId: file.value.folder_id } };
  }
  return auth.user?.role === 'link' ? null : { name: 'SharedWithMe' };
});

watch(
  [() => fileId, () => auth.user?.session_id] as const,
  async ([id]) => {
    const version = ++requestVersion;
    file.value = null;
    showShare.value = false;
    error.value = '';
    loading.value = true;
    try {
      const current = await fileManager.fetchFile(id);
      if (version === requestVersion) {
        file.value = current;
      }
    } catch {
      if (version === requestVersion) {
        error.value = 'This file is unavailable. Access may have expired or been removed.';
      }
    } finally {
      if (version === requestVersion) {
        loading.value = false;
      }
    }
  },
  { immediate: true },
);
onBeforeUnmount(() => {
  requestVersion += 1;
});
</script>

<template>
  <p v-if="loading" role="status">Loading file…</p>
  <p v-else-if="error" role="alert">{{ error }}</p>
  <section v-else-if="file" class="file-detail">
    <header class="flex flex-wrap items-center justify-between gap-3 mb-4">
      <h1 class="text-2xl font-semibold break-words min-w-0">{{ file.filename }}</h1>
      <div class="flex flex-wrap items-center gap-2">
        <Button
          v-if="file.capabilities?.can_share && auth.user?.role !== 'link'"
          label="Share"
          icon="oi oi-users"
          aria-haspopup="dialog"
          :aria-expanded="showShare"
          :aria-controls="`sharing-item-${file.id}`"
          @click="showShare = true"
        />
        <Button
          v-if="homeRoute"
          as="router-link"
          :to="homeRoute"
          icon="oi oi-arrow-left"
          label="Back"
          severity="secondary"
          outlined
          size="small"
        />
        <Button
          v-if="file.capabilities?.can_read"
          as="a"
          :href="downloadUrl"
          :download="file?.filename"
          icon="oi oi-download"
          label="Download"
          severity="info"
          size="small"
        />
      </div>
    </header>
    <FilePreview
      v-if="file.capabilities?.can_read"
      :file-type="getFileType(file)"
      :file-url="downloadUrl"
      maximized
      class="pt-2"
    />
    <SharingDialog
      v-if="showShare"
      :visible="showShare"
      :scope="{ kind: 'item', id: file.id, name: file.filename, itemType: file.type }"
      @close="showShare = false"
    />
  </section>
</template>

<style scoped>
.file-detail {
  min-width: 0;
  color: var(--p-text-color);
}
</style>
