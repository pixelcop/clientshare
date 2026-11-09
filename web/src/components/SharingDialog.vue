<script setup lang="ts">
import { Button, Dialog } from 'openvue';

import type { SharingScope } from '@/types/sharing';

import SharingPanel from './SharingPanel.vue';
const { visible, scope } = defineProps<{ visible: boolean; scope: SharingScope }>();
const emit = defineEmits<{ close: [] }>();
const composing = ref(false);
</script>
<template>
  <Dialog
    :id="`sharing-${scope.kind}-${scope.id}`"
    :visible="visible"
    :header="`Share: ${scope.name}`"
    modal
    blockScroll
    :draggable="false"
    :style="{ width: '36rem', maxWidth: '95vw' }"
    @update:visible="(value) => !value && emit('close')"
  >
    <SharingPanel
      v-if="visible"
      :key="`${scope.kind}-${scope.id}`"
      :scope
      @composing="composing = $event"
    />
    <template v-if="!composing" #footer
      ><Button
        label="Done"
        severity="secondary"
        :style="{ minHeight: '44px' }"
        @click="emit('close')"
    /></template>
  </Dialog>
</template>
