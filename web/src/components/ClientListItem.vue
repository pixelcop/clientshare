<script setup lang="ts">
import { breakpointsTailwind, useBreakpoints } from '@vueuse/core';
import { SplitButton } from 'primevue';
import type { MenuItem } from 'primevue/menuitem';

import type { Client } from '@/types/models';

const { client, showLinks, showUsers } = defineProps<{
  client: Client;
  showLinks?: boolean;
  showUsers?: boolean;
}>();
const emit = defineEmits(['edit', 'delete', 'add-user']);

const router = useRouter();
const breakpoints = useBreakpoints(breakpointsTailwind);
const xs = breakpoints.smaller('sm');

function onClickFiles() {
  if (client.root_folder_id) {
    void router.push(`/folder/${client.root_folder_id}`);
  }
}

function onClickUsers() {
  void router.push({
    name: 'Users',
    query: { client_id: client.id },
  });
}

function onClickAddUser() {
  emit('add-user', client);
}

const clientActions = computed<MenuItem[]>(() => [
  {
    label: 'View Files',
    icon: 'pi pi-folder',
    command: () => onClickFiles(),
  },
  {
    label: 'View Users',
    icon: 'pi pi-users',
    command: () => onClickUsers(),
    visible: showUsers,
  },
  {
    label: 'Add User',
    icon: 'pi pi-user-plus',
    command: () => onClickAddUser(),
    visible: showUsers,
  },
  {
    label: 'Links',
    icon: 'pi pi-link',
    url: `/clients/${client.id}/links`,
    visible: showLinks,
  },
  {
    label: 'Edit',
    icon: 'pi pi-pencil',
    command: () => emit('edit', client),
    visible: showLinks,
  },
  {
    label: 'Delete',
    icon: 'pi pi-trash',
    command: () => emit('delete', client),
    visible: showLinks,
  },
]);
</script>
<template>
  <tr>
    <td class="client-name px-0 pr-4 sm:px-4 py-2" @click="onClickFiles">
      <router-link v-if="client.root_folder_id" :to="`/folder/${client.root_folder_id}`">{{
        client.name
      }}</router-link>
      <span v-else>{{ client.name }}</span>
    </td>
    <!--
    <td class="px-4 py-2 hidden sm:table-cell">{{ client.folder_path }}</td>
    <td class="px-4 py-2 hidden sm:table-cell">{{ formatDate(client.created_at) }}</td>
    -->
    <td class="px-0 sm:px-4 py-2">
      <div class="flex justify-end">
        <SplitButton
          :model="clientActions"
          icon="pi pi-folder"
          :label="xs ? undefined : 'View Files'"
          severity="info"
          class="view-files"
          @click.stop="onClickFiles"
        />
      </div>
    </td>
  </tr>
</template>

<style scoped>
@reference '@/assets/tailwind.css';

:deep(.view-files .p-button-label) {
  @apply hidden;
}

:deep(.view-files .p-button-label) {
  @apply sm:inline;
}

.client-name {
  cursor: pointer;
}
</style>
