<script setup lang="ts">
import { useClientsStore } from '../stores/clients';

const clientsStore = useClientsStore();

onMounted(() => {
  clientsStore.fetchClients();
});
</script>

<template>
  <div>
    <h1 class="text-2xl font-bold mb-4">All Client Folders</h1>
    <table class="min-w-full bg-white rounded shadow">
      <thead>
        <tr>
          <th class="px-4 py-2 text-left">Name</th>
          <th class="px-4 py-2 text-left">Folder Path</th>
          <th class="px-4 py-2 text-left">Actions</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="client in clientsStore.clients" :key="client.id">
          <td class="px-4 py-2">{{ client.name }}</td>
          <td class="px-4 py-2">{{ client.folder_path }}</td>
          <td class="px-4 py-2">
            <router-link :to="`/clients/${client.id}/files`" class="text-blue-600 hover:underline"
              >Open</router-link
            >
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
table {
  border-collapse: collapse;
}
th,
td {
  border-bottom: 1px solid #eee;
}
</style>
