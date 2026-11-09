<script setup lang="ts">
import { Button, Dialog } from 'openvue';

import { useAuthStore } from '@/stores/auth';

import SharingPanel from './SharingPanel.vue';

defineProps<{ visible: boolean; clientId: string; clientName: string }>();
const emit = defineEmits<{ close: [] }>();
const auth = useAuthStore();
const canManagePeople = computed(() => ['admin', 'manager'].includes(auth.user?.role || ''));
const canManageLinks = computed(() => ['admin', 'manager'].includes(auth.user?.role || ''));
</script>

<template>
  <Dialog
    :visible="visible"
    :header="`Manage client access: ${clientName}`"
    :aria-label="`Manage client access: ${clientName}`"
    modal
    blockScroll
    position="right"
    :draggable="false"
    :style="{ width: '42rem', maxWidth: '95vw' }"
    @update:visible="(value) => !value && emit('close')"
  >
    <SharingPanel
      v-if="visible"
      :key="clientId"
      :scope="{ kind: 'client', id: clientId, name: clientName }"
    />
    <template #footer>
      <div class="flex flex-wrap items-center gap-2 w-full">
        <Button v-if="canManagePeople" asChild v-slot="slotProps" severity="secondary">
          <router-link
            :class="slotProps.class"
            :to="{ name: 'Users', query: { client_id: clientId } }"
            @click="emit('close')"
            >Manage people</router-link
          >
        </Button>
        <Button v-if="canManageLinks" asChild v-slot="slotProps" severity="secondary">
          <router-link
            :class="slotProps.class"
            :to="`/clients/${clientId}/links`"
            @click="emit('close')"
            >Manage links</router-link
          >
        </Button>
        <Button label="Done" severity="secondary" class="ml-auto" @click="emit('close')" />
      </div>
    </template>
  </Dialog>
</template>

<style scoped>
.client-access {
  display: grid;
  gap: 16px;
  font-size: 14px;
}
.access-scope {
  font-weight: 600;
  color: var(--p-text-color);
}
.access-note,
.client-access section p {
  color: var(--p-text-muted-color);
  line-height: 1.5;
}
.client-access section {
  padding-top: 16px;
  border-top: 1px solid var(--p-content-border-color);
}
.client-access h3 {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 18px;
  font-weight: 600;
  margin: 0 0 8px;
}
.client-access section p {
  margin-bottom: 12px;
}
.client-access :deep(.p-button) {
  min-height: 44px;
}
</style>
