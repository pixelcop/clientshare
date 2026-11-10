<script setup lang="ts">
import { Button, Dialog } from 'openvue';

import SharingPanel from './SharingPanel.vue';

defineProps<{ visible: boolean; clientId: string; clientName: string }>();
const emit = defineEmits<{ close: [] }>();
</script>

<template>
  <Dialog
    :visible="visible"
    :header="`Share workspace: ${clientName}`"
    :aria-label="`Share workspace: ${clientName}`"
    modal
    blockScroll
    :draggable="false"
    :pt="{ header: { style: { paddingBottom: '12px' } } }"
    :style="{ width: '36rem', maxWidth: '95vw' }"
    @update:visible="(value) => !value && emit('close')"
  >
    <SharingPanel
      v-if="visible"
      :key="clientId"
      :scope="{ kind: 'client', id: clientId, name: clientName }"
    >
      <template #done>
        <Button
          label="Done"
          severity="secondary"
          :style="{ minHeight: '44px' }"
          @click="emit('close')"
        />
      </template>
    </SharingPanel>
  </Dialog>
</template>
