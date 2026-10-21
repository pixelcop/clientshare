<script setup lang="ts">
import axios from 'axios';
import { onMounted, ref } from 'vue';

interface Usage {
  staff_users: number;
  max_users: number;
  storage_bytes: number;
  reserved_bytes: number;
  max_storage_bytes: number;
}
const usage = ref<Usage | null>(null);
const error = ref('');
onMounted(async () => {
  try {
    usage.value = (await axios.get<Usage>('/api/settings/usage')).data;
  } catch {
    error.value = 'Unable to load workspace usage.';
  }
});
</script>

<template>
  <section class="rounded-lg border border-surface-200 p-4 mb-6" aria-label="Workspace usage">
    <h2 class="text-lg font-semibold">Workspace usage</h2>
    <p v-if="error" role="status">{{ error }}</p>
    <template v-else-if="usage">
      <p>
        Staff: {{ usage.staff_users }} of {{ usage.max_users || 'unlimited' }} (including pending
        invitations)
      </p>
      <p>
        Storage: {{ (usage.storage_bytes / 1e9).toFixed(2) }} GB of
        {{ usage.max_storage_bytes ? `${usage.max_storage_bytes / 1e9} GB` : 'unlimited' }}
      </p>
      <p v-if="usage.reserved_bytes">
        Uploads in progress: {{ (usage.reserved_bytes / 1e9).toFixed(2) }} GB
      </p>
      <p class="text-sm">Client users are unlimited.</p>
      <p
        v-if="
          (usage.max_users > 0 && usage.staff_users >= usage.max_users) ||
          (usage.max_storage_bytes > 0 && usage.storage_bytes >= usage.max_storage_bytes)
        "
        class="mt-2 text-amber-700"
      >
        A configured limit has been reached. Remove staff or files, or contact your administrator to
        increase the limit. Existing access remains available.
      </p>
    </template>
    <p v-else>Loading usage…</p>
  </section>
</template>
