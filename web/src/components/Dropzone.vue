<script setup lang="ts">
import { useMediaQuery } from '@vueuse/core';
import { Button, Dialog, InputText, ProgressBar, SplitButton } from 'openvue';
import type { MenuItem } from 'openvue/menuitem';
import type { ComponentPublicInstance } from 'vue';

import { useUploadController } from '@/composables/useUploadController';
import { useWorkspacePresentation } from '@/composables/useWorkspacePresentation';
import { useFileManagerStore } from '@/stores/fileManager';

import NewFolderModal from './NewFolderModal.vue';
import PendingEmail from './PendingEmail.vue';

const props = withDefaults(defineProps<{ canNotify?: boolean; compactActions?: MenuItem[] }>(), {
  canNotify: true,
  compactActions: () => [],
});
const compact = useMediaQuery('(max-width: 639px)');
const emit = defineEmits<{ uploaded: [] }>();
const fileManager = useFileManagerStore();
const presentation = useWorkspacePresentation();
const notifyClient = ref(true);
const pending = ref<InstanceType<typeof PendingEmail> | null>(null);
const canManageEmailQueue = computed(() => !!fileManager.context?.can_manage_queue);
const canToggleNotifications = computed(() => props.canNotify && presentation.value.canWrite);
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
  onUploaded: () => {
    void pending.value?.fetchPendingEmails();
    emit('uploaded');
  },
});
const uploadActions = computed<MenuItem[]>(() => [
  { label: 'Upload folder', icon: 'oi oi-folder', command: chooseDirectory },
  ...(compact.value
    ? [
        {
          label: 'New folder',
          icon: 'oi oi-folder-plus',
          disabled: fileManager.loading || !fileManager.canWrite,
          command: () => (showNewFolder.value = true),
        },
        ...props.compactActions,
      ]
    : []),
]);
const destination = computed(() => fileManager.context?.folder.filename || 'this folder');
function setFileInput(element: Element | ComponentPublicInstance | null) {
  fileInput.value = element instanceof HTMLInputElement ? element : null;
}
function setDirectoryInput(element: Element | ComponentPublicInstance | null) {
  directoryInput.value = element instanceof HTMLInputElement ? element : null;
}
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
      <div class="workspace-context">
        <slot name="context" />
        <span
          v-if="fileManager.loading && presentation.loaded"
          class="workspace-status"
          role="status"
          >Loading…</span
        >
      </div>
      <div id="collect-workspace-actions" class="workspace-actions">
        <template v-if="presentation.canWrite">
          <SplitButton
            label="Upload"
            icon="oi oi-upload"
            class="workspace-upload"
            :model="uploadActions"
            :disabled="busy || fileManager.loading || !fileManager.canWrite"
            :menuButtonProps="{ 'aria-label': 'More file actions' }"
            @click="onClick"
          />
          <Button
            v-if="!compact"
            label="New folder"
            icon="oi oi-folder-plus"
            severity="secondary"
            :disabled="fileManager.loading || !fileManager.canWrite"
            @click="showNewFolder = true"
          />
        </template>
        <slot v-if="!compact || !presentation.canWrite" name="actions" />
        <PendingEmail
          v-if="fileManager.context?.client_id && (canManageEmailQueue || canToggleNotifications)"
          ref="pending"
          v-model:notificationsEnabled="notifyClient"
          :clientId="fileManager.context.client_id"
          :canManageQueue="canManageEmailQueue"
          :canNotify="canToggleNotifications"
          :disabled="fileManager.loading"
          :notificationsDisabled="busy"
        />
      </div>
    </div>
    <input
      v-if="fileManager.canWrite"
      :ref="setFileInput"
      type="file"
      hidden
      multiple
      aria-label="Upload files"
      @change="onFileChange"
    />
    <input
      v-if="fileManager.canWrite"
      :ref="setDirectoryInput"
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
    <div
      id="collect-files"
      class="workspace-content"
      :aria-busy="fileManager.loading"
      :inert="fileManager.loading"
    >
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
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  flex: 1 1 180px;
}
.workspace-status {
  color: var(--p-text-muted-color);
  font-size: 12px;
}
.workspace-actions {
  flex-wrap: wrap;
}
.workspace-actions :deep(button) {
  min-height: 44px;
}
@media (max-width: 639px) {
  .file-workspace {
    padding: 16px;
  }
  .workspace-context {
    flex-basis: 100%;
    flex-wrap: wrap;
  }
  .workspace-actions {
    width: 100%;
  }
  .workspace-upload {
    flex: 1;
    min-width: 0;
  }
  .workspace-upload :deep(.p-splitbutton-button) {
    flex: 1;
  }
}
.workspace-content {
  position: relative;
  margin-top: 16px;
  min-height: 180px;
  transition: opacity 160ms ease-out;
}
.workspace-content[aria-busy='true'] {
  opacity: 0.6;
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
