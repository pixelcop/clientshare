<script setup lang="ts">
import axios from 'axios';
import { Button } from 'openvue';
import { storeToRefs } from 'pinia';

import ClientAccessDialog from '@/components/ClientAccessDialog.vue';
import ClientWorkspaceSwitcher from '@/components/ClientWorkspaceSwitcher.vue';
import CollectionInvitation from '@/components/CollectionInvitation.vue';
import FileList from '@/components/FileList.vue';
import PendingEmail from '@/components/PendingEmail.vue';
import { useClientsStore } from '@/stores/clients';

import { useAuthStore } from '../stores/auth';
import { useFileManagerStore } from '../stores/fileManager';

const { folderId } = defineProps<{ folderId?: string }>();
const router = useRouter();

const fileManager = useFileManagerStore();
const { clientId } = storeToRefs(useClientsStore());

const { user } = storeToRefs(useAuthStore());

const pending = ref<InstanceType<typeof PendingEmail> | null>(null);
const showAccess = ref(false);
const destinationError = ref('');

const canManageAccess = computed(() => !!fileManager.context?.client_capabilities?.can_share);
const canNotify = computed(() => user.value?.role !== 'customer');
const canManageEmailQueue = computed(() => !!fileManager.context?.can_manage_queue);

function handleUploaded() {
  if (canManageEmailQueue.value && pending.value) {
    pending.value.fetchPendingEmails();
  }
}

let destinationVersion = 0;
async function openDestination(id = folderId) {
  const version = ++destinationVersion;
  destinationError.value = '';
  showAccess.value = false;
  if (id) {
    await fileManager.changeFolder(id);
    return;
  }
  await fileManager.changeFolder();
  try {
    const response = await axios.get<{ context: { folder: { id: string } } }>('/api/folders');
    if (version === destinationVersion) {
      await router.replace(`/folder/${response.data.context.folder.id}`);
    }
  } catch (error) {
    if (version !== destinationVersion) {
      return;
    }
    const status = axios.isAxiosError(error) ? error.response?.status : undefined;
    if (status === 404 || (status === 400 && user.value?.role === 'admin')) {
      await router.replace(user.value?.role === 'admin' ? '/clients' : '/shared-with-me');
    } else {
      destinationError.value = 'The workspace could not be loaded. Please try again.';
    }
  }
}
watch(
  () => folderId,
  (id) => {
    void openDestination(id);
  },
  { immediate: true },
);
onBeforeUnmount(() => {
  destinationVersion += 1;
});
</script>

<template>
  <div>
    <div class="flex flex-wrap items-center justify-between gap-3 mb-4">
      <h1 class="text-2xl md:text-[28px] font-semibold min-w-0 break-words">
        {{
          fileManager.context?.client_name || fileManager.context?.folder.filename || 'File Manager'
        }}
      </h1>
      <div class="flex flex-wrap items-center gap-3 w-full sm:w-auto">
        <ClientWorkspaceSwitcher :clientId="fileManager.context?.client_id" />
        <Button
          v-if="canManageAccess && clientId"
          :id="`collect-links-${clientId}`"
          label="Manage client access"
          icon="oi oi-users"
          severity="secondary"
          @click="showAccess = true"
        />
      </div>
    </div>

    <div v-if="destinationError" role="alert">
      <p>{{ destinationError }}</p>
      <Button label="Try again" severity="secondary" @click="openDestination()" />
    </div>
    <FileList v-else :canNotify @uploaded="handleUploaded">
      <template #actions>
        <PendingEmail v-if="canManageEmailQueue && clientId" :clientId ref="pending" />
      </template>
      <template #empty>
        <CollectionInvitation v-if="clientId && canManageAccess" :clientId="clientId" compact />
      </template>
    </FileList>
    <ClientAccessDialog
      v-if="clientId && canManageAccess"
      :visible="showAccess"
      :clientId
      :clientName="fileManager.context?.client_name || 'Client workspace'"
      @close="showAccess = false"
    />
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
