<script setup lang="ts">
import { useResizeObserver } from '@vueuse/core';
import { Button } from 'openvue';

import { imageClickZoom, imageGeometry, stepImageZoom } from '@/utils/imagePreview';

const { fileUrl, fileName } = defineProps<{ fileUrl: string; fileName: string }>();
const emit = defineEmits<{ error: [] }>();
const stage = ref<HTMLElement | null>(null);
const target = ref<HTMLButtonElement | null>(null);
const naturalSize = ref({ width: 0, height: 0 });
const viewport = ref({ width: 1, height: 1 });
const quarterTurns = ref(0);
const fitMode = ref(true);
const manualScale = ref(1);
const ready = computed(() => naturalSize.value.width > 0 && naturalSize.value.height > 0);
const geometry = computed(() => imageGeometry(
  naturalSize.value.width, naturalSize.value.height,
  viewport.value.width, viewport.value.height, quarterTurns.value,
));
const scale = computed(() => fitMode.value ? geometry.value.fitScale : manualScale.value);
const percentage = computed(() => {
  const percent = scale.value * 100;
  return `${Number(percent.toFixed(percent < 10 ? 1 : 0))}%`;
});
const clickScale = computed(() => imageClickZoom(scale.value, geometry.value));
const willReset = computed(() => clickScale.value < scale.value);
const resetLabel = computed(() => geometry.value.fitScale < 1 ? 'Reset zoom to fit' : 'Reset zoom to 100%');
const clickLabel = computed(() => willReset.value
  ? resetLabel.value
  : geometry.value.fitScale < 1 ? 'Zoom image to 100%' : 'Zoom in on image');
const targetStyle = computed(() => ({
  width: `${geometry.value.width * scale.value}px`,
  height: `${geometry.value.height * scale.value}px`,
}));
const imageStyle = computed(() => ({
  width: `${naturalSize.value.width * scale.value}px`,
  height: `${naturalSize.value.height * scale.value}px`,
  transform: `translate(-50%, -50%) rotate(${-quarterTurns.value * 90}deg)`,
}));

function measure() {
  if (!stage.value) {return;}
  const style = getComputedStyle(stage.value);
  viewport.value = {
    width: Math.max(1, stage.value.clientWidth - (parseFloat(style.paddingLeft) || 0) - (parseFloat(style.paddingRight) || 0)),
    height: Math.max(1, stage.value.clientHeight - (parseFloat(style.paddingTop) || 0) - (parseFloat(style.paddingBottom) || 0)),
  };
}
useResizeObserver(stage, measure);

function loadImage(event: Event) {
  const image = event.target as HTMLImageElement;
  naturalSize.value = { width: image.naturalWidth, height: image.naturalHeight };
  measure();
}

// Preserve the image point under the pointer (or viewport center for controls).
async function changeZoom(next: number, event?: MouseEvent) {
  if (!ready.value || !stage.value || !target.value) {return;}
  const stageRect = stage.value.getBoundingClientRect();
  const rect = target.value.getBoundingClientRect();
  const x = event?.detail ? event.clientX : stageRect.left + stage.value.clientWidth / 2;
  const y = event?.detail ? event.clientY : stageRect.top + 32 + viewport.value.height / 2;
  const point = {
    x: rect.width ? Math.max(0, Math.min(1, (x - rect.left) / rect.width)) : 0.5,
    y: rect.height ? Math.max(0, Math.min(1, (y - rect.top) / rect.height)) : 0.5,
  };
  fitMode.value = Math.abs(next - geometry.value.fitScale) < 0.00001;
  manualScale.value = next;
  await nextTick();
  if (!stage.value || !target.value) {return;}
  const updated = target.value.getBoundingClientRect();
  stage.value.scrollLeft += updated.left + updated.width * point.x - x;
  stage.value.scrollTop += updated.top + updated.height * point.y - y;
}

function zoomBy(direction: 1 | -1) {
  void changeZoom(stepImageZoom(scale.value, direction, geometry.value));
}

function resetZoom() {
  void changeZoom(geometry.value.fitScale);
}

function clickImage(event: MouseEvent) {
  void changeZoom(clickScale.value, event);
}

async function rotateLeft() {
  quarterTurns.value = (quarterTurns.value + 1) % 4;
  await nextTick();
  if (!stage.value) {return;}
  stage.value.scrollLeft = (stage.value.scrollWidth - stage.value.clientWidth) / 2;
  stage.value.scrollTop = (stage.value.scrollHeight - stage.value.clientHeight) / 2;
}

watch(() => fileUrl, () => {
  naturalSize.value = { width: 0, height: 0 };
  quarterTurns.value = 0;
  fitMode.value = true;
  manualScale.value = 1;
  if (stage.value) {
    stage.value.scrollLeft = 0;
    stage.value.scrollTop = 0;
  }
});
</script>

<template>
  <div class="image-preview">
    <div ref="stage" class="image-stage">
      <button
        ref="target"
        type="button"
        class="image-target"
        :class="{ 'is-ready': ready, 'will-reset': willReset }"
        :style="targetStyle"
        :aria-label="clickLabel"
        :title="clickLabel"
        :tabindex="ready ? 0 : -1"
        @click="clickImage"
      >
        <img :key="fileUrl" :src="fileUrl" :alt="fileName" :style="imageStyle" draggable="false" @load="loadImage" @error="emit('error')" />
      </button>
    </div>
    <p v-if="!ready" class="image-loading" role="status">Loading image…</p>
    <div v-else class="image-controls" role="group" aria-label="Image controls">
      <div class="image-zoom-controls">
        <Button unstyled icon="oi oi-minus" aria-label="Zoom out" title="Zoom out" :disabled="scale <= geometry.minScale" @click="zoomBy(-1)" />
        <Button
          unstyled
          class="image-zoom-value"
          :label="percentage"
          :aria-label="`Zoom ${percentage}. ${resetLabel}; double-click or press Enter.`"
          :title="`${resetLabel} (double-click)`"
          @dblclick.prevent="resetZoom"
          @keydown.enter.prevent="resetZoom"
          @keydown.space.prevent="resetZoom"
        />
        <Button unstyled icon="oi oi-plus" aria-label="Zoom in" title="Zoom in" :disabled="scale >= geometry.maxScale" @click="zoomBy(1)" />
      </div>
      <Button unstyled icon="oi oi-undo" aria-label="Rotate left" title="Rotate left 90° — preview only" @click="rotateLeft" />
      <span class="sr-only" role="status">Zoom {{ percentage }}. Rotated {{ quarterTurns * 90 }} degrees left.</span>
    </div>
  </div>
</template>

<style scoped>
.image-preview {
  position: relative;
  width: 100%;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}
.image-stage {
  display: flex;
  align-items: flex-start;
  width: 100%;
  height: 100%;
  overflow: auto;
  padding: 32px 32px 104px;
  scrollbar-color: var(--p-surface-400) transparent;
}
.image-target {
  position: relative;
  flex: none;
  margin: auto;
  padding: 0;
  border: 0;
  background: transparent;
  overflow: hidden;
  visibility: hidden;
  cursor: zoom-in;
  user-select: none;
}
.image-target.is-ready {
  visibility: visible;
}
.image-target.will-reset {
  cursor: zoom-out;
}
.image-target img {
  position: absolute;
  top: 50%;
  left: 50%;
  max-width: none;
  max-height: none;
}
.image-loading {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  margin: 0;
  color: var(--p-text-muted-color);
  pointer-events: none;
}
.image-controls {
  position: absolute;
  bottom: max(24px, env(safe-area-inset-bottom));
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px;
  border-radius: 999px;
  background: var(--p-surface-700);
  color: var(--p-surface-0);
  box-shadow: 0 4px 16px rgb(0 0 0 / 12%);
}
.image-zoom-controls {
  display: flex;
  align-items: center;
  border-radius: 999px;
  background: color-mix(in srgb, currentColor 12%, transparent);
}
.image-controls :deep(button) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  width: 44px;
  height: 44px;
  padding: 0;
  border: 0;
  border-radius: 999px;
  background: transparent;
  color: inherit;
  cursor: pointer;
  touch-action: manipulation;
}
.image-controls :deep(button:hover:not(:disabled)) {
  background: color-mix(in srgb, currentColor 16%, transparent);
}
.image-controls :deep(button:active:not(:disabled)) {
  background: color-mix(in srgb, currentColor 24%, transparent);
}
.image-controls :deep(button:disabled) {
  opacity: 0.4;
  cursor: default;
}
.image-controls :deep(.image-zoom-value) {
  width: auto;
  min-width: 64px;
  padding: 0 8px;
  font-size: 14px;
  font-weight: 500;
  font-variant-numeric: tabular-nums;
}
.image-controls :deep(.oi) {
  font-size: 18px;
}
.image-controls :deep(button:focus-visible) {
  outline: 2px solid currentColor;
  outline-offset: -3px;
}
.image-target:focus-visible {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 4px;
}
@media (hover: hover) and (pointer: fine) and (min-width: 641px) {
  .image-controls {
    opacity: 0;
    pointer-events: none;
    transition: opacity 160ms ease-out;
  }
  .image-preview:hover .image-controls,
  .image-preview:focus-within .image-controls {
    opacity: 1;
    pointer-events: auto;
  }
}
@media (prefers-reduced-motion: reduce) {
  .image-controls {
    transition: none;
  }
}
</style>
