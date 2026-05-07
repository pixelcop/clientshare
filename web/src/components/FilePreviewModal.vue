<script setup lang="ts">
import { breakpointsTailwind, useBreakpoints } from '@vueuse/core';
import { storeToRefs } from 'pinia';
import { Button, Dialog } from 'primevue';

import { useFile } from '@/composables/useFile';
import { useBrandingStore } from '@/stores/branding';
import type { FileItem } from '@/types/models';

import FilePreview from './FilePreview.vue';

const { file, fileType, visible } = defineProps<{
  file: FileItem;
  fileUrl: string;
  fileType: string;
  visible: boolean;
}>();

const dialog = ref<InstanceType<typeof Dialog> | null>(null);
const emit = defineEmits(['close', 'download']);

const breakpoints = useBreakpoints(breakpointsTailwind);
const xs = breakpoints.smaller('sm');
const maximized = ref(false);
const brandingStore = useBrandingStore();
const { branding } = storeToRefs(brandingStore);

const { isPdf, isText, isImage } = useFile(fileType);

const maximizable = computed(() => {
  return isPdf || isText || isImage;
});

const absoluteFileLink = computed(() => {
  const baseURL = branding.value.effectivePublicBaseURL.trim();
  if (baseURL) {
    return `${baseURL.replace(/\/+$/, '')}/files/${file.id}`;
  }
  return new URL(`/files/${file.id}`, window.location.origin).toString();
});

function close() {
  emit('close');
}

function onShowDialog() {
  if (dialog.value && xs.value) {
    // start maximized on small screens
    dialog.value.maximized = true;
  }
}

const copyIcon = ref('las la-link');
let resetCopyIconTimeout: ReturnType<typeof setTimeout> | null = null;

async function onClickCopyLink(e: PointerEvent) {
  e.preventDefault();
  const url = absoluteFileLink.value;
  if (!url) {
    return;
  }
  try {
    await navigator.clipboard.writeText(url);
    if (resetCopyIconTimeout) {
      clearTimeout(resetCopyIconTimeout);
    }
    copyIcon.value = 'las la-check';
    resetCopyIconTimeout = setTimeout(() => {
      copyIcon.value = 'las la-link';
    }, 2000);
  } catch {
    // no-op
  }
}
</script>

<template>
  <Dialog
    ref="dialog"
    :visible="visible"
    @update:visible="
      (val) => {
        if (!val) close();
      }
    "
    @maximize="() => (maximized = true)"
    @unmaximize="() => (maximized = false)"
    @show="onShowDialog"
    modal
    header="File Preview"
    :maximizable
    closable
    dismissableMask
    blockScroll
    class="xl:min-w-2/5 md:min-w-3/5 sm:min-w-4/5 min-w-5/6"
  >
    <template #header>
      <div class="flex flex-row items-stretch w-full @container">
        <div class="p-dialog-title w-full">File Preview</div>
        <div class="mr-2 flex gap-2">
          <!-- class="ml-auto [&_.p-button-label]:hidden @sm:[&_.p-button-label]:inline" -->
          <Button
            label="Download"
            icon="pi pi-download"
            size="small"
            severity="info"
            @click="$emit('download')"
          />

          <Button class="copy" title="Copy link" severity="secondary" @click="onClickCopyLink">
            <Transition name="copy-icon-swap" mode="out-in">
              <i :key="copyIcon" :class="copyIcon" class="copy-icon" />
            </Transition>
          </Button>
        </div>
      </div>
    </template>

    <FilePreview :file-url="fileUrl" :file-type="fileType" :maximized />
  </Dialog>
</template>

<style scoped>
.copy {
  background-color: transparent;
  border: 0;
  padding: 0;
}

:deep(.copy) span {
  font-size: 1.5rem;
}

.copy-icon {
  display: inline-block;
  font-size: 1.5rem;
}

/* Animation when changing icon */
.copy-icon-swap-enter-active,
.copy-icon-swap-leave-active {
  transition:
    opacity 90ms ease,
    transform 110ms cubic-bezier(0.2, 0.7, 0.2, 1);
}

.copy-icon-swap-enter-from,
.copy-icon-swap-leave-to {
  opacity: 0;
  transform: scale(0.75) rotate(-10deg);
}

@media (prefers-reduced-motion: reduce) {
  .copy-icon-swap-enter-active,
  .copy-icon-swap-leave-active {
    transition: none;
  }
}
</style>
