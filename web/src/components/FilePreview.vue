<script setup lang="ts">
import { PDFViewer, type PDFViewerConfig, ZoomMode } from '@embedpdf/vue-pdf-viewer';

import { useFile } from '@/composables/useFile';

const { fileType, fileUrl, maximized } = defineProps<{
  fileUrl: string;
  fileType: string;
  maximized?: boolean;
}>();

const { isAudio, isPdf, isText, isImage } = useFile(fileType);

const textContent = ref('');

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

async function loadTextContent() {
  if (!isText) {
    return;
  }
  fetch(fileUrl)
    .then((res) => res.text())
    .then((text) => {
      textContent.value = text;
    })
    .catch((e) => {
      textContent.value = 'Error loading file.';
    });
}

onMounted(() => {
  if (isText && !textContent.value) {
    void loadTextContent();
  }
});
</script>

<template>
  <div v-if="isImage">
    <img :src="fileUrl" alt="Image preview" class="max-h-96 mx-auto" />
  </div>
  <div v-else-if="isAudio" class="flex flex-col items-center">
    <audio :src="fileUrl" controls class="w-full max-w-lg">
      Your browser does not support the audio element.
    </audio>
  </div>
  <div v-else-if="isPdf" class="h-full">
    <PDFViewer class="pdf" :config="pdfConfig" :style="pdfSize" />
  </div>
  <div v-else-if="isText">
    <pre class="bg-gray-100 p-4 rounded overflow-auto max-h-96"><code>{{ textContent }}</code></pre>
  </div>
  <div v-else>
    <div class="flex flex-col items-center justify-center">
      <span class="mb-4">Preview not supported for this file type.</span>
      <Button :href="fileUrl" download>Download</Button>
    </div>
  </div>
</template>

<style scoped>
.pdf {
  width: 100vw;
  height: 100vh;
}
</style>
