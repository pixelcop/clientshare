<script setup lang="ts">
import { Button, Dialog } from 'openvue';

import type { SharingScope } from '@/types/sharing';

import SharingPanel from './SharingPanel.vue';
const { visible, scope } = defineProps<{ visible: boolean; scope: SharingScope }>();
const emit = defineEmits<{ close: [] }>();
</script>
<template>
  <Dialog
    :id="`sharing-${scope.kind}-${scope.id}`"
    :visible="visible"
    :header="`${scope.kind === 'client' ? 'Manage client access' : 'Share'}: ${scope.name}`"
    modal
    blockScroll
    :draggable="false"
    :style="{ width: '42rem', maxWidth: '95vw' }"
    @update:visible="(value) => !value && emit('close')"
  >
    <SharingPanel v-if="visible" :key="`${scope.kind}-${scope.id}`" :scope />
    <template #footer><Button label="Done" severity="secondary" @click="emit('close')" /></template>
  </Dialog>
</template>
