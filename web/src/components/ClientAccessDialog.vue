<script setup lang="ts">
import { Dialog } from 'openvue';

import SharingPanel from './SharingPanel.vue';

defineProps<{ visible: boolean; clientId: string; clientName: string }>();
const emit = defineEmits<{ close: [] }>();
const panel = ref<InstanceType<typeof SharingPanel> | null>(null);
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
    :visible="visible"
    :header="`Share workspace: ${clientName}`"
    :aria-label="`Share workspace: ${clientName}`"
    modal
    dismissableMask
    blockScroll
    :draggable="false"
    :pt="{ header: { style: { paddingBottom: '12px' } } }"
    :style="{ width: '36rem', maxWidth: '95vw' }"
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
