<script setup lang="ts">
import { FloatLabel, type SelectChangeEvent } from 'openvue';
import { storeToRefs } from 'pinia';

import Breadcrumb from '@/components/Breadcrumb.vue';
import FileList from '@/components/FileList.vue';
import { replaySensitiveSelectPT } from '@/services/replayPrivacy';
import { useClientsStore } from '@/stores/clients';
import { useFileManagerStore } from '@/stores/fileManager';

const { folderId } = defineProps<{ folderId?: string }>();

const router = useRouter();
const fileManager = useFileManagerStore();
const clientsStore = useClientsStore();
const { currentClient, clients, clientId } = storeToRefs(clientsStore);
const { currentFolder } = storeToRefs(fileManager);


async function handleClientChange(event: SelectChangeEvent) {
  const selectedId = event.value;
  if (!selectedId) {
    return;
  }

  clientId.value = selectedId as string;
  if (currentClient.value?.root_folder_id) {
    void router.push(`/folder/${currentClient.value!.root_folder_id}`);
  }
}

async function initializeFolder() {
  if (folderId) {
    await fileManager.changeFolder(folderId);
    return;
  }

  // Keep current folder if already set; only auto-pick root on first nav with no folder context.
  if (currentFolder.value) {
    return;
  }

  if (clientsStore.clients.length === 0) {
    await clientsStore.fetchClients();
  }

  const firstClientRootFolderId = clientsStore.clients[0]?.root_folder_id;
  if (firstClientRootFolderId) {
    clientsStore.clientId = clientsStore.clients[0].id;
    void router.push(`/folder/${firstClientRootFolderId}`);
  }
}

onMounted(() => {
  void initializeFolder();
});

watch(
  () => folderId,
  (id) => {
    if (id) {
      void fileManager.changeFolder(id);
    }
  },
);
</script>

<template>
  <div class="space-y-6">
    <section class="bg-white rounded-xl shadow-sm border border-slate-200 p-6">
      <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
        <Breadcrumb heading class="min-w-0 flex-1" />

        <div v-if="clients.length > 1" class="shrink-0 md:ml-auto md:text-right">
          <FloatLabel>
            <Select
              :pt="replaySensitiveSelectPT"
              id="client-select"
              v-model="clientId"
              :options="clients"
              optionLabel="name"
              optionValue="id"
              @change="handleClientChange"
            />
            <label for="client-select">Select Client </label>
          </FloatLabel>
        </div>
      </div>
    </section>

    <section class="space-y-4">
      <FileList />
    </section>
  </div>
</template>
