<script setup lang="ts">
import { useMediaQuery } from '@vueuse/core';
import { Dialog } from 'openvue';

import type { SharingScope } from '@/types/sharing';

import SharingPanel from './SharingPanel.vue';
const { visible, scope } = defineProps<{ visible: boolean; scope: SharingScope }>();
const emit = defineEmits<{ close: [] }>();
const panel = ref<InstanceType<typeof SharingPanel> | null>(null);
const isMobile = useMediaQuery('(max-width: 639px)');
function requestClose() {
  if (panel.value) {
    panel.value.requestClose();
  } else {
    emit('close');
  }
}
</script>
<template>
  <Dialog
    class="sharing-dialog"
    :showHeader="!isMobile"
    :aria-label="`Share${scope.kind === 'item' && scope.itemType === 'folder' ? ' folder' : ''}: ${scope.name}`"
    :id="`sharing-${scope.kind}-${scope.id}`"
    :visible="visible"
    :header="`Share${scope.kind === 'item' && scope.itemType === 'folder' ? ' folder' : ''}: ${scope.name}`"
    modal
    dismissableMask
    blockScroll
    :draggable="false"
    :pt="{ header: { style: { paddingBottom: '12px' } } }"
    :style="{ width: '44rem', maxWidth: '95vw' }"
    @update:visible="(value) => !value && requestClose()"
  >
    <SharingPanel
      v-if="visible"
      ref="panel"
      :key="`${scope.kind}-${scope.id}`"
      :scope
      @close="emit('close')"
    />
  </Dialog>
</template>
