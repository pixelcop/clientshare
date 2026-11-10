<script setup lang="ts">
import axios from 'axios';
import { Button } from 'openvue';
import { storeToRefs } from 'pinia';

import Breadcrumb from '@/components/Breadcrumb.vue';
import ClientAccessDialog from '@/components/ClientAccessDialog.vue';
import ClientWorkspaceSwitcher from '@/components/ClientWorkspaceSwitcher.vue';
import CollectionInvitation from '@/components/CollectionInvitation.vue';
import FileList from '@/components/FileList.vue';
import PendingEmail from '@/components/PendingEmail.vue';
import SharingDialog from '@/components/SharingDialog.vue';
import { useWorkspacePresentation } from '@/composables/useWorkspacePresentation';
import { useClientsStore } from '@/stores/clients';

import { useAuthStore } from '../stores/auth';
import { useFileManagerStore } from '../stores/fileManager';

const { folderId } = defineProps<{ folderId?: string }>();
const router = useRouter();

const fileManager = useFileManagerStore();
const presentation = useWorkspacePresentation();
const displayIsClientRoot = computed(() => {
  const context = presentation.value.context;
  return (
    !!context &&
    Object.values(context.client_capabilities).some(Boolean) &&
    context.folder.id === context.root_folder_id
  );
});
const displayCanShare = computed(() => {
  const context = presentation.value.context;
  return displayIsClientRoot.value
    ? !!context?.client_capabilities.can_share
    : !!context?.capabilities.can_share;
});
const { clientId } = storeToRefs(useClientsStore());

const { user } = storeToRefs(useAuthStore());

const pending = ref<InstanceType<typeof PendingEmail> | null>(null);
const showAccess = ref(false);
const showFolderShare = ref(false);
const destinationError = ref('');

const canManageAccess = computed(() => !!fileManager.context?.client_capabilities?.can_share);
const isClientRoot = computed(
  () =>
    !!fileManager.context &&
    Object.values(fileManager.context.client_capabilities).some(Boolean) &&
    fileManager.context.folder.id === fileManager.context.root_folder_id,
);
const canShareCurrent = computed(() =>
  isClientRoot.value ? canManageAccess.value : !!fileManager.context?.capabilities.can_share,
);
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
  showFolderShare.value = false;
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
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center mb-4">
      <Breadcrumb heading class="min-w-0 flex-1" />
      <div class="flex flex-wrap items-center gap-3 w-full sm:w-auto">
        <ClientWorkspaceSwitcher :clientId="presentation.context?.client_id" />
      </div>
    </div>

    <div v-if="destinationError" role="alert">
      <p>{{ destinationError }}</p>
      <Button label="Try again" severity="secondary" @click="openDestination()" />
    </div>
    <FileList v-else :canNotify @uploaded="handleUploaded">
      <template #actions>
        <Button
          v-if="displayCanShare"
          :id="`collect-links-${clientId}`"
          :label="displayIsClientRoot ? 'Share workspace' : 'Share folder'"
          icon="oi oi-users"
          severity="secondary"
          :disabled="fileManager.loading || !canShareCurrent"
          @click="isClientRoot ? (showAccess = true) : (showFolderShare = true)"
        />
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
    <SharingDialog
      v-if="!isClientRoot && fileManager.context?.capabilities.can_share"
      :visible="showFolderShare"
      :scope="{
        kind: 'item',
        id: fileManager.context.folder.id,
        name: fileManager.context.folder.filename,
        itemType: 'folder',
      }"
      @close="showFolderShare = false"
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
