<script setup lang="ts">
import { PDFViewer, type PDFViewerConfig, ZoomMode } from '@embedpdf/vue-pdf-viewer';

import { useFile } from '@/composables/useFile';

const {
  fileType,
  fileUrl,
  downloadUrl,
  fileName = 'File',
  maximized,
} = defineProps<{
  fileUrl: string;
  downloadUrl: string;
  fileType: string;
  maximized?: boolean;
  fileName?: string;
}>();

const { isAudio, isPdf, isText, isImage } = useFile(fileType);

const textContent = ref('');
const loading = ref(false);
const failed = ref(false);

const pdfSize = computed(() => {
  return maximized
    ? 'width: 100%; height: 100%;'
    : 'width: 75vw; max-width: 90vw; max-height: 80vh;';
});

const pdfConfig = computed<PDFViewerConfig>(() => {
  if (!isPdf) {
    return {};
  }
  return {
    src: fileUrl,
    zoom: {
      defaultZoomLevel: ZoomMode.Automatic,
    },
    disabledCategories: ['annotation', 'redaction', 'panel-comment'],
    theme: {
      preference: 'light', // 'light' | 'dark' | 'system'
    },
  };
});

watch(
  () => fileUrl,
  async (url, _previous, onCleanup) => {
    const controller = new AbortController();
    onCleanup(() => controller.abort());
    failed.value = false;
    textContent.value = '';
    if (!isText) {return;}
    loading.value = true;
    try {
      const response = await fetch(url, { signal: controller.signal });
      if (!response.ok) {throw new Error('Preview request failed');}
      const text = await response.text();
      if (!controller.signal.aborted) {textContent.value = text;}
    } catch {
      if (!controller.signal.aborted) {failed.value = true;}
    } finally {
      if (!controller.signal.aborted) {loading.value = false;}
    }
  },
  { immediate: true },
);
</script>

<template>
  <div v-if="failed || (!isImage && !isAudio && !isPdf && !isText)" class="preview-fallback">
    <i class="oi oi-file" aria-hidden="true" />
    <h2>{{ failed ? 'Preview couldn’t be loaded' : 'No preview available' }}</h2>
    <p>Download the file to open it on your device.</p>
    <Button as="a" :href="downloadUrl" :download="fileName" label="Download" icon="oi oi-download" />
  </div>
  <div v-else-if="isImage" class="image-preview" :class="{ maximized }">
    <img :src="fileUrl" :alt="fileName" @error="failed = true" />
  </div>
  <div v-else-if="isAudio" class="audio-preview">
    <audio :src="fileUrl" controls class="w-full max-w-lg" @error="failed = true">
      Your browser does not support the audio element.
    </audio>
  </div>
  <div v-else-if="isPdf" class="pdf-preview">
    <PDFViewer class="pdf" :config="pdfConfig" :style="pdfSize" />
  </div>
  <div v-else-if="isText" class="text-preview" :class="{ maximized }">
    <p v-if="loading" role="status">Loading preview…</p>
    <pre v-else><code>{{ textContent }}</code></pre>
  </div>
</template>

<style scoped>
.pdf-preview {
  width: 100%;
  height: 100%;
  min-height: 0;
}
.audio-preview,
.preview-fallback {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: center;
  padding: 32px;
}
.image-preview {
  display: flex;
  align-items: flex-start;
  width: 100%;
  min-width: 0;
  min-height: 0;
  overflow: auto;
  padding: 32px;
}
.image-preview img {
  display: block;
  flex: none;
  margin: auto;
  width: auto;
  height: auto;
  max-width: none;
  max-height: none;
}
.text-preview {
  width: 100%;
  overflow: auto;
  max-height: 60vh;
  background: var(--p-content-background);
  padding: 24px;
}
.text-preview.maximized {
  max-height: none;
}
.text-preview pre {
  margin: 0;
  font-size: 14px;
  line-height: 1.7;
  tab-size: 4;
}
.preview-fallback {
  flex-direction: column;
  gap: 16px;
  text-align: center;
}
.preview-fallback > i {
  font-size: 72px;
  color: var(--p-text-muted-color);
}
.preview-fallback h2 {
  font-size: 20px;
  font-weight: 600;
}
.preview-fallback p {
  color: var(--p-text-muted-color);
}
</style>
