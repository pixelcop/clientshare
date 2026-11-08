<script setup lang="ts">
import { Button } from 'openvue';
import { storeToRefs } from 'pinia';

import CollectionInvitation from '@/components/CollectionInvitation.vue';
import FileList from '@/components/FileList.vue';
import PendingEmail from '@/components/PendingEmail.vue';
import { useClientsStore } from '@/stores/clients';

import { useAuthStore } from '../stores/auth';
import { useFileManagerStore } from '../stores/fileManager';

const { folderId } = defineProps<{ folderId: string }>();

const fileManager = useFileManagerStore();
const { clientId } = storeToRefs(useClientsStore());

const { user } = storeToRefs(useAuthStore());

const pending = ref<InstanceType<typeof PendingEmail> | null>(null);

const showLinks = computed(() => user.value?.role === 'admin');
const showUsers = computed(() => ['admin', 'manager'].includes(user.value?.role ?? ''));
const canNotify = computed(() => user.value?.role !== 'customer');
const canManageEmailQueue = computed(() => ['admin', 'manager'].includes(user.value?.role ?? ''));

function handleUploaded() {
  if (canManageEmailQueue.value && pending.value) {
    pending.value.fetchPendingEmails();
  }
}

onMounted(() => {
  void fileManager.changeFolder(folderId);
  if (canManageEmailQueue.value && pending.value) {
    pending.value.fetchPendingEmails();
  }
});

watch(
  () => folderId,
  (id) => {
    fileManager.changeFolder(id);
    if (canManageEmailQueue.value && pending.value) {
      pending.value.fetchPendingEmails();
    }
  },
);
</script>

<template>
  <div>
    <div class="flex items-center justify-between mb-4">
      <h1>{{ fileManager.context?.client_name || 'File Manager' }}</h1>
      <div class="flex items-center gap-2">
        <Button
          v-if="showUsers && clientId"
          asChild
          v-slot="slotProps"
          severity="secondary"
          class="border-slate-300"
        >
          <router-link
            :class="slotProps.class"
            :to="{ name: 'Users', query: { client_id: clientId } }"
          >
            Users
          </router-link>
        </Button>

        <Button
          v-if="showLinks && clientId"
          asChild
          v-slot="slotProps"
          severity="secondary"
          class="border-slate-300"
        >
          <router-link
            :id="`collect-links-${clientId}`"
            :class="slotProps.class"
            :to="`/clients/${clientId}/links`"
          >
            Secure Links
          </router-link>
        </Button>
      </div>
    </div>

    <FileList :canNotify @uploaded="handleUploaded">
      <template #actions>
        <PendingEmail v-if="canManageEmailQueue && clientId" :clientId ref="pending" />
      </template>
      <template #empty>
        <CollectionInvitation v-if="clientId" :clientId="clientId" compact />
      </template>
    </FileList>
  </div>
</template>

<style scoped>
table {
  border-collapse: collapse;
  background-color: var(--p-dialog-background);
  padding: 4px;
}
th,
td {
  border-bottom: 1px solid var(--p-dialog-border-color);
}

:deep(.panel) .p-panel-header {
  display: none !important;
}
</style>
