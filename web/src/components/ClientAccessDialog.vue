<script setup lang="ts">
import { useMediaQuery } from '@vueuse/core';
import { Dialog } from 'openvue';

import SharingPanel from './SharingPanel.vue';

defineProps<{ visible: boolean; clientId: string; clientName: string }>();
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
    :visible="visible"
    :header="`Share workspace: ${clientName}`"
    :aria-label="`Share workspace: ${clientName}`"
    modal
    dismissableMask
    blockScroll
    :draggable="false"
    :pt="{ title: { 'data-sentry-mask': true }, header: { style: { paddingBottom: '12px' } } }"
    :style="{ width: '44rem', maxWidth: '95vw' }"
    @update:visible="(value) => !value && requestClose()"
  >
    <SharingPanel
      v-if="visible"
      ref="panel"
      :key="clientId"
      :scope="{ kind: 'client', id: clientId, name: clientName }"
      @close="emit('close')"
    />
  </Dialog>
</template>
