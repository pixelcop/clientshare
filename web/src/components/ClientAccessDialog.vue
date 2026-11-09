<script setup lang="ts">
import { Button, Dialog } from 'openvue';

import SharingPanel from './SharingPanel.vue';

defineProps<{ visible: boolean; clientId: string; clientName: string }>();
const emit = defineEmits<{ close: [] }>();
const composing = ref(false);
</script>

<template>
  <Dialog
    :visible="visible"
    :header="`Share: ${clientName}`"
    :aria-label="`Share: ${clientName}`"
    modal
    blockScroll
    :draggable="false"
    :style="{ width: '36rem', maxWidth: '95vw' }"
    @update:visible="(value) => !value && emit('close')"
  >
    <SharingPanel
      v-if="visible"
      :key="clientId"
      :scope="{ kind: 'client', id: clientId, name: clientName }"
      @composing="composing = $event"
    />
    <template v-if="!composing" #footer>
      <Button
        label="Done"
        severity="secondary"
        :style="{ minHeight: '44px' }"
        @click="emit('close')"
      />
    </template>
  </Dialog>
</template>
