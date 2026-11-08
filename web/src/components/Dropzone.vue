<script setup lang="ts">
import { Button, Dialog, InputText, ProgressBar, SplitButton } from 'openvue';

import { useUploadController } from '@/composables/useUploadController';
import { useFileManagerStore } from '@/stores/fileManager';

import NewFolderModal from './NewFolderModal.vue';

const props = withDefaults(defineProps<{ canNotify?: boolean }>(), { canNotify: true });
const emit = defineEmits<{ uploaded: [] }>();
const fileManager = useFileManagerStore();
const notifyClient = ref(true);
const showNewFolder = ref(false);
const {
  fileInput,
  directoryInput,
  dragging,
  busy,
  stopped,
  stopUpload,
  uploading,
  uploadProgress,
  uploadError,
  showRename,
  renameItems,
  renameSaving,
  renameError,
  resetRenameState,
  submitRename,
  onDragOver,
  onDragLeave,
  onDragEnd,
  onDrop,
  onClick,
  chooseDirectory,
  onFileChange,
  onDirectoryChange,
} = useUploadController({
  canNotify: () => props.canNotify,
  notifyClient: () => notifyClient.value,
  onUploaded: () => emit('uploaded'),
});
const uploadActions = [{ label: 'Upload folder', icon: 'oi oi-folder', command: chooseDirectory }];
const destination = computed(() => fileManager.context?.folder.filename || 'this folder');
</script>

<template>
  <section
    class="file-workspace"
    aria-label="Files"
    @dragover="onDragOver"
    @dragleave="onDragLeave"
    @dragend="onDragEnd"
    @drop="onDrop"
  >
    <div class="workspace-toolbar">
      <div class="workspace-context"><slot name="context" /></div>
      <div class="workspace-actions">
        <template v-if="fileManager.canWrite">
          <SplitButton
            label="Upload"
            icon="oi oi-upload"
            :model="uploadActions"
            :disabled="busy || !fileManager.context"
            :menuButtonProps="{ 'aria-label': 'More upload actions' }"
            @click="onClick"
          />
          <Button
            label="New folder"
            icon="oi oi-folder-plus"
            severity="secondary"
            :disabled="!fileManager.context"
            @click="showNewFolder = true"
          />
        </template>
        <slot name="actions" />
      </div>
    </div>
    <div v-if="props.canNotify && fileManager.canWrite" class="upload-options">
      <details>
        <summary>Upload options · Email notifications {{ notifyClient ? 'on' : 'off' }}</summary>
        <label class="upload-notification">
          <input v-model="notifyClient" type="checkbox" :disabled="busy" />
          Notify by email after upload
        </label>
      </details>
    </div>
    <input
      v-if="fileManager.canWrite"
      ref="fileInput"
      type="file"
      hidden
      multiple
      aria-label="Upload files"
      @change="onFileChange"
    />
    <input
      v-if="fileManager.canWrite"
      ref="directoryInput"
      type="file"
      hidden
      webkitdirectory
      multiple
      aria-label="Upload folder"
      @change="onDirectoryChange"
    />
    <div v-if="uploading" class="upload-progress" role="status">
      <div class="flex items-center justify-between gap-3">
        <span
          >{{ stopped ? 'Stopping upload…' : 'Uploading…' }} {{ Math.round(uploadProgress) }}%</span
        >
        <Button
          v-if="!stopped && uploadProgress < 100"
          label="Stop upload"
          severity="secondary"
          @click="stopUpload"
        />
      </div>
      <ProgressBar :value="uploadProgress" :showValue="false" aria-label="Upload progress" />
    </div>
    <p v-if="uploadError" class="upload-error" role="alert">{{ uploadError }}</p>
    <div class="workspace-content">
      <slot />
      <div v-if="dragging" class="upload-highlight" aria-live="polite">
        <i class="oi oi-upload" aria-hidden="true" />
        <span>Drop files to upload to {{ destination }}</span>
      </div>
    </div>
    <Dialog
      :visible="showRename"
      modal
      :closable="!renameSaving"
      :closeOnEscape="!renameSaving"
      header="Rename duplicate files"
      class="w-[calc(100%-2rem)] max-w-xl"
      @update:visible="(visible) => !visible && resetRenameState()"
    >
      <form class="space-y-4" @submit.prevent="submitRename">
        <p>These files already exist in the upload destination. Rename them to continue.</p>
        <div v-for="item in renameItems" :key="item.id" class="space-y-1">
          <label :for="item.id" class="text-sm">{{ item.originalName }}</label>
          <InputText
            :id="item.id"
            v-model="item.name"
            class="w-full"
            :disabled="renameSaving"
            required
          />
        </div>
        <p v-if="renameError" class="upload-error" role="alert">{{ renameError }}</p>
        <div class="flex justify-end gap-2">
          <Button
            label="Cancel"
            severity="secondary"
            :disabled="renameSaving"
            @click="resetRenameState"
          />
          <Button type="submit" label="Upload" :loading="renameSaving" />
        </div>
      </form>
    </Dialog>
    <NewFolderModal
      v-if="showNewFolder && fileManager.canWrite"
      :show="showNewFolder"
      @close="showNewFolder = false"
    />
  </section>
</template>

<style scoped>
.file-workspace {
  border: 1px solid var(--p-content-border-color);
  border-radius: 12px;
  background: var(--p-content-background);
  color: var(--p-content-color);
  padding: 24px;
}
.workspace-toolbar,
.workspace-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.workspace-toolbar {
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
}
.workspace-context {
  min-width: 0;
  flex: 1 1 180px;
}
.workspace-actions {
  flex-wrap: wrap;
}
.workspace-actions :deep(button) {
  min-height: 44px;
}
.workspace-content {
  position: relative;
  margin-top: 16px;
  min-height: 180px;
}
.upload-options {
  margin-top: 8px;
  font-size: 12px;
  color: var(--p-text-muted-color);
}
.upload-options summary {
  cursor: pointer;
  padding-block: 8px;
}
.upload-notification {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 44px;
}
.upload-progress {
  margin-top: 16px;
  display: grid;
  gap: 8px;
  font-variant-numeric: tabular-nums;
  font-size: 14px;
}
.upload-error {
  color: var(--p-red-600);
  margin-block: 12px;
  font-size: 14px;
}
.upload-highlight {
  position: absolute;
  inset: 0;
  z-index: 2;
  pointer-events: none;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  border: 2px dashed var(--p-primary-color);
  border-radius: 8px;
  background: color-mix(in srgb, var(--p-content-background) 94%, var(--p-primary-color));
  color: var(--p-content-color);
  font-weight: 600;
}
.upload-highlight .oi {
  font-size: 28px;
}
@media (max-width: 640px) {
  .file-workspace {
    padding: 16px;
  }
}
</style>
