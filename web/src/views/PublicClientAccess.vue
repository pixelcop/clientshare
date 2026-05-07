<script setup lang="ts">
import { storeToRefs } from 'pinia';
import { FloatLabel, type SelectChangeEvent } from 'primevue';

import Dropzone from '@/components/Dropzone.vue';
import FileList from '@/components/FileList.vue';
import { useClientsStore } from '@/stores/clients';
import { useFileManagerStore } from '@/stores/fileManager';

const { folderId } = defineProps<{ folderId?: string }>();

const router = useRouter();
const fileManager = useFileManagerStore();
const clientsStore = useClientsStore();
const { currentClient, clients, clientId } = storeToRefs(clientsStore);
const { currentFolder } = storeToRefs(fileManager);

const clientName = computed(() => currentClient.value?.name || 'Client Files');

function handleUploaded() {}

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
        <div>
          <h1 class="text-2xl font-semibold text-slate-900">{{ clientName }}</h1>
          <p class="text-sm text-slate-500">Secure access</p>
        </div>

        <div v-if="clients.length > 1" class="md:ml-auto md:text-right">
          <FloatLabel>
            <Select
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
      <div class="space-y-3 mb-6">
        <Dropzone @uploaded="handleUploaded" notifyClient canNotify />
      </div>

      <FileList />
    </section>
  </div>
</template>
