<script setup lang="ts">
import { storeToRefs } from 'pinia';
import { Button } from 'primevue';

import FileList from '@/components/FileList.vue';
import PendingEmail from '@/components/PendingEmail.vue';
import { useClientsStore } from '@/stores/clients';

import Dropzone from '../components/Dropzone.vue';
import { useAuthStore } from '../stores/auth';
import { useFileManagerStore } from '../stores/fileManager';

const { folderId } = defineProps<{ folderId: string }>();

const fileManager = useFileManagerStore();
const { currentClient, clientId } = storeToRefs(useClientsStore());

const { user } = storeToRefs(useAuthStore());
const notifyClient = ref(true);

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
      <h1>{{ currentClient?.name || 'File Manager' }}</h1>
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
          <router-link :class="slotProps.class" :to="`/clients/${clientId}/links`">
            Secure Links
          </router-link>
        </Button>
      </div>
    </div>

    <Dropzone @uploaded="handleUploaded" :notifyClient :canNotify />

    <div v-if="canNotify" class="flex flex-row justify-between mt-4">
      <div class="flex items-center gap-2 text-sm">
        <input id="notify-client" v-model="notifyClient" type="checkbox" class="h-4 w-4" />
        <label for="notify-client">Notify client by email after upload</label>
      </div>

      <PendingEmail v-if="canManageEmailQueue && clientId" :clientId ref="pending" />
    </div>

    <FileList />
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
