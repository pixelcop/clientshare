<script setup lang="ts">
import axios from 'axios';
import { storeToRefs } from 'pinia';
import type { PageState } from 'primevue';
import { Button } from 'primevue';

import { useTenantSettingsStore } from '@/stores/tenantSettings';
import type { Client } from '@/types/models';

import ClientListItem from '../components/ClientListItem.vue';
import ClientModal from '../components/ClientModal.vue';
import DeleteClientModal from '../components/DeleteClientModal.vue';
import UserModal from '../components/UserModal.vue';
import { useAuthStore } from '../stores/auth';
import { useClientsStore } from '../stores/clients';
import { useSecureLinksStore } from '../stores/secureLinks';
import type { UserCreatePayload } from '../stores/users';
import { useUsersStore } from '../stores/users';

type CreateClientPayload = {
  name: string;
  generateLink: boolean;
  sendEmail: boolean;
  accessType: 'read' | 'write' | 'readwrite';
  expiryDays: number;
  email: string;
};

const clientsStore = useClientsStore();
const secureLinksStore = useSecureLinksStore();
const tenantSettingsStore = useTenantSettingsStore();
const authStore = useAuthStore();
const usersStore = useUsersStore();
const { listedClients, listingPagination, listingLoading } = storeToRefs(clientsStore);
const { error: usersError, loading: usersLoading } = storeToRefs(usersStore);
const { settings: tenantSettings } = storeToRefs(tenantSettingsStore);
const search = ref('');
const showCreate = ref(false);
const showCreateUser = ref(false);
const showEdit = ref(false);
const showDelete = ref(false);
const editName = ref('');
const editClientObj = ref<Client | null>(null);
const createUserClient = ref<Client | null>(null);
const deleteClientObj = ref<Client | null>(null);
const createLinkError = ref<string | null>(null);
const pageSizeOptions = [10, 20, 50];
const createUserDefaultClientIds = computed(() =>
  createUserClient.value ? [createUserClient.value.id] : [],
);
const first = computed(() =>
  Math.max(0, (listingPagination.value.page - 1) * listingPagination.value.page_size),
);
let searchDebounceTimeout: ReturnType<typeof setTimeout> | null = null;

onMounted(() => {
  void clientsStore.fetchClientPage(1, listingPagination.value.page_size, search.value);
  void clientsStore.fetchClients();
  void tenantSettingsStore.loadTenantSettings().catch(() => undefined);
});

onBeforeUnmount(() => {
  if (searchDebounceTimeout) {
    clearTimeout(searchDebounceTimeout);
  }
});

watch(search, (value) => {
  if (searchDebounceTimeout) {
    clearTimeout(searchDebounceTimeout);
  }
  searchDebounceTimeout = setTimeout(() => {
    void clientsStore.fetchClientPage(1, listingPagination.value.page_size, value);
  }, 300);
});

const showLinks = computed(() => authStore.user?.role === 'admin');
const showUsers = computed(() => ['admin', 'manager'].includes(authStore.user?.role ?? ''));

const handlePageChange = (event: PageState) => {
  void clientsStore.fetchClientPage(event.page + 1, event.rows, search.value);
};

async function create(payload: CreateClientPayload | { name: string }) {
  if (!('generateLink' in payload)) {
    return;
  }
  createLinkError.value = null;
  const client = await clientsStore.createClient(payload.name);
  showCreate.value = false;
  if (payload.generateLink) {
    try {
      await secureLinksStore.createLink(
        client.id,
        payload.accessType,
        payload.expiryDays,
        payload.sendEmail ? payload.email : '',
      );
    } catch (e: unknown) {
      createLinkError.value =
        (axios.isAxiosError(e) ? e.response?.data?.error : null) ||
        secureLinksStore.error ||
        'Failed to create secure link';
    }
  }
}

function editClient(client: Client) {
  createLinkError.value = null;
  editClientObj.value = client;
  editName.value = client.name;
  showEdit.value = true;
}

function update(payload: { name: string }) {
  if (!editClientObj.value) {
    return;
  }
  clientsStore.updateClient(editClientObj.value.id, payload.name).then(() => {
    showEdit.value = false;
    editName.value = '';
    editClientObj.value = null;
  });
}

function confirmDelete(client: Client) {
  createLinkError.value = null;
  deleteClientObj.value = client;
  showDelete.value = true;
}

function openCreateUser(client: Client) {
  usersStore.error = null;
  createUserClient.value = client;
  showCreateUser.value = true;
}

function closeCreateUser() {
  usersStore.error = null;
  showCreateUser.value = false;
  createUserClient.value = null;
}

async function createUser(payload: UserCreatePayload) {
  await usersStore.createUser(payload);
  closeCreateUser();
}

function remove() {
  if (!deleteClientObj.value) {
    return;
  }
  clientsStore.deleteClient(deleteClientObj.value.id).then(() => {
    showDelete.value = false;
    deleteClientObj.value = null;
  });
}
</script>

<template>
  <div>
    <div class="flex items-center justify-between mb-4">
      <h1 class="text-2xl font-bold">Clients</h1>
      <Button
        @click="
          createLinkError = null;
          showCreate = true;
        "
      >
        New Client
      </Button>
    </div>
    <div
      v-if="createLinkError"
      class="mb-4 rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700"
    >
      {{ createLinkError }}
    </div>
    <InputText
      v-model="search"
      type="text"
      placeholder="Search clients..."
      class="mb-4 px-3 py-2 rounded w-full sm:max-w-md"
    />

    <Card class="mt-4 shadow">
      <template #header>
        <div class="px-4 pt-3 border-b border-surface flex items-center justify-between">
          <h2 class="font-semibold text-slate-900">Client Directory</h2>
        </div>
      </template>
      <template #content>
        <table v-if="listedClients.length > 0" class="min-w-full">
          <thead>
            <tr>
              <th class="px-0 pr-4 sm:px-4 py-2 text-left">Name</th>
              <!--
              <th class="px-4 py-2 text-left hidden sm:table-cell">Folder Path</th>
              <th class="px-4 py-2 text-left hidden sm:table-cell">Created</th>
              -->
              <th class="px-0 sm:px-4 py-2"></th>
            </tr>
          </thead>
          <tbody>
            <ClientListItem
              v-for="client in listedClients"
              :key="client.id"
              :client="client"
              :showLinks="showLinks"
              :showUsers="showUsers"
              @add-user="openCreateUser"
              @edit="editClient"
              @delete="confirmDelete"
            />
          </tbody>
        </table>
        <div v-else-if="!listingLoading" class="text-sm text-muted-color">No clients found.</div>
      </template>
      <template v-if="listingPagination.total_items > 0 && !listingLoading" #footer>
        <div class="flex items-center justify-between pt-3 border-t border-surface">
          <div class="text-sm text-muted-color">
            Showing {{ (listingPagination.page - 1) * listingPagination.page_size + 1 }} to
            {{
              Math.min(
                listingPagination.page * listingPagination.page_size,
                listingPagination.total_items,
              )
            }}
            of
            {{ listingPagination.total_items }}
          </div>
          <Paginator
            :alwaysShow="false"
            :first="first"
            :rows="listingPagination.page_size"
            :totalRecords="listingPagination.total_items"
            :rowsPerPageOptions="pageSizeOptions"
            @page="handlePageChange"
          />
        </div>
      </template>
    </Card>

    <ClientModal
      :show="showCreate"
      mode="add"
      :error="clientsStore.error ?? undefined"
      :linkError="createLinkError"
      :defaultSecureLinkExpiryDays="tenantSettings.secure_link_default_expiry_days"
      @close="showCreate = false"
      @submit="create"
    />

    <ClientModal
      :show="showEdit"
      mode="edit"
      :initialName="editName"
      :error="clientsStore.error ?? undefined"
      @close="showEdit = false"
      @submit="update"
    />

    <DeleteClientModal
      :show="showDelete"
      :clientName="deleteClientObj?.name || ''"
      :error="clientsStore.error ?? undefined"
      @close="showDelete = false"
      @delete="remove"
    />

    <UserModal
      :show="showCreateUser"
      mode="add"
      :clients="clientsStore.clients"
      :defaultRole="'client'"
      :defaultClientIds="createUserDefaultClientIds"
      :error="usersError ?? undefined"
      :loading="usersLoading"
      @close="closeCreateUser"
      @submit="createUser"
    />
  </div>
</template>
