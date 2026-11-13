<script setup lang="ts">
import axios from 'axios';
import { Button, Select } from 'openvue';

import { replaySensitiveSelectPT } from '@/services/replayPrivacy';
import { useAuthStore } from '@/stores/auth';
import type { Client } from '@/types/models';

const { clientId } = defineProps<{ clientId?: string | null }>();
const auth = useAuthStore();
const router = useRouter();
const id = useId();
const workspaces = ref<Client[]>([]);
const loading = ref(false);
const switching = ref(false);
const error = ref('');
let version = 0;
const isClientAccount = computed(() => ['client', 'customer'].includes(auth.user?.role || ''));
const selected = computed(
  () => workspaces.value.find((client) => client.id === clientId)?.id ?? null,
);

async function load() {
  const current = ++version;
  workspaces.value = [];
  error.value = '';
  loading.value = isClientAccount.value;
  if (!isClientAccount.value) {
    return;
  }
  try {
    // This endpoint excludes item-only grants. Never infer a workspace option
    // from the current folder, breadcrumbs or Shared with me results.
    const response = await axios.get<Client[]>('/api/clients');
    if (current === version) {
      workspaces.value = (response.data || []).filter((client) => client.root_folder_id);
    }
  } catch {
    if (current === version) {
      error.value = 'Client workspaces could not be loaded.';
    }
  } finally {
    if (current === version) {
      loading.value = false;
    }
  }
}

async function switchWorkspace(value: string) {
  const client = workspaces.value.find((item) => item.id === value);
  if (!client?.root_folder_id || switching.value) {
    return;
  }
  switching.value = true;
  error.value = '';
  try {
    // The route change loads fresh authorized context/capabilities; do not set
    // the shared file store's client ID ahead of that response.
    await router.push({ name: 'FolderView', params: { folderId: client.root_folder_id } });
  } catch {
    error.value = 'The selected workspace could not be opened.';
  } finally {
    switching.value = false;
  }
}

watch(
  [
    () => auth.user?.tenant_id,
    () => auth.user?.id,
    () => auth.user?.role,
    () => auth.user?.session_id,
  ],
  load,
  { immediate: true },
);
onBeforeUnmount(() => {
  version += 1;
});
</script>

<template>
  <div v-if="isClientAccount && (loading || error || workspaces.length)" class="workspace-switcher">
    <label :id="`${id}-label`" :for="id">Client workspace</label>
    <Select
      :pt="replaySensitiveSelectPT"
      :inputId="id"
      :ariaLabelledby="`${id}-label`"
      :modelValue="selected"
      :options="workspaces"
      optionLabel="name"
      optionValue="id"
      :placeholder="loading ? 'Loading workspaces…' : 'Choose a client workspace'"
      :loading="loading"
      :disabled="loading || switching || !workspaces.length"
      class="min-h-11"
      fluid
      @update:modelValue="switchWorkspace"
    />
    <div v-if="error" role="alert">
      <p>{{ error }}</p>
      <Button label="Retry workspaces" severity="secondary" :disabled="loading" @click="load" />
    </div>
  </div>
</template>

<style scoped>
.workspace-switcher {
  display: grid;
  gap: 4px;
  width: min(100%, 20rem);
}
label {
  color: var(--p-text-muted-color);
  font-size: 12px;
  font-weight: 500;
}
[role='alert'] {
  color: var(--p-text-color);
  font-size: 12px;
  line-height: 1.5;
}
@media (min-width: 640px) {
  .workspace-switcher {
    width: 16rem;
  }
}
</style>
