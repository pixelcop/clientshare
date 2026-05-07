<script setup lang="ts">
import { storeToRefs } from 'pinia';
import type { PageState } from 'primevue';
import { Button } from 'primevue';
import { useToast } from 'primevue/usetoast';

import DeleteUserModal from '@/components/DeleteUserModal.vue';
import UserListItem from '@/components/UserListItem.vue';
import UserModal from '@/components/UserModal.vue';
import { useClientsStore } from '@/stores/clients';
import type { GeneratedUserAccessLink } from '@/stores/users';
import type { UserListFilters } from '@/stores/users';
import type { UserCreatePayload, UserUpdatePayload } from '@/stores/users';
import { useUsersStore } from '@/stores/users';
import type { User } from '@/types/models';

const props = withDefaults(
  defineProps<{
    clientId?: string;
  }>(),
  {
    clientId: '',
  },
);

const usersStore = useUsersStore();
const clientsStore = useClientsStore();
const { listedUsers, listingPagination, listingLoading } = storeToRefs(usersStore);
const toast = useToast();
const router = useRouter();

const search = ref('');
const showCreate = ref(false);
const showEdit = ref(false);
const showDelete = ref(false);
const editUserObj = ref<User | null>(null);
const deleteUserObj = ref<User | null>(null);
const copyingLinkUserId = ref<string | null>(null);
const resendingUserId = ref<string | null>(null);
const resendingAllInvites = ref(false);
const pageSizeOptions = [10, 20, 50];
const inviteStatusOptions = [
  { label: 'All invite statuses', value: '' },
  { label: 'Accepted invite', value: 'accepted' },
  { label: 'Pending invite', value: 'pending' },
];
const roleOptions = [
  { label: 'All roles', value: '' },
  { label: 'Admin', value: 'admin' },
  { label: 'Manager', value: 'manager' },
];
const selectedInviteStatus = ref('');
const selectedRole = ref('');
const scopedClientId = computed(() => props.clientId.trim());
const scopedClient = computed(
  () => clientsStore.clients.find((client) => client.id === scopedClientId.value) ?? null,
);
const createUserDefaultRole = computed<'admin' | 'manager' | 'client' | undefined>(() =>
  scopedClientId.value ? 'client' : undefined,
);
const createUserDefaultClientIds = computed(() =>
  scopedClientId.value ? [scopedClientId.value] : [],
);
const first = computed(() =>
  Math.max(0, (listingPagination.value.page - 1) * listingPagination.value.page_size),
);
let searchDebounceTimeout: ReturnType<typeof setTimeout> | null = null;

const currentFilters = computed<UserListFilters>(() => {
  return {
    client_id: scopedClientId.value,
    invite_status: selectedInviteStatus.value,
    role: selectedRole.value,
  };
});

function fetchUserPage(page = 1, pageSize = listingPagination.value.page_size) {
  return usersStore.fetchUserPage(page, pageSize, search.value, currentFilters.value);
}

onMounted(() => {
  void fetchUserPage(1, listingPagination.value.page_size);
  void clientsStore.fetchClients();
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
    void usersStore.fetchUserPage(
      1,
      listingPagination.value.page_size,
      value,
      currentFilters.value,
    );
  }, 300);
});

watch(currentFilters, () => {
  void fetchUserPage(1, listingPagination.value.page_size);
});

const handlePageChange = (event: PageState) => {
  void fetchUserPage(event.page + 1, event.rows);
};

function editUser(user: User) {
  editUserObj.value = user;
  showEdit.value = true;
}

function confirmDelete(user: User) {
  deleteUserObj.value = user;
  showDelete.value = true;
}

async function createUser(payload: UserCreatePayload) {
  await usersStore.createUser(payload);
  showCreate.value = false;
}

async function updateUser(payload: UserUpdatePayload) {
  if (!editUserObj.value) {
    return;
  }
  await usersStore.updateUser(editUserObj.value.id, payload);
  showEdit.value = false;
  editUserObj.value = null;
}

async function removeUser() {
  if (!deleteUserObj.value) {
    return;
  }
  await usersStore.deleteUser(deleteUserObj.value.id);
  showDelete.value = false;
  deleteUserObj.value = null;
}

async function resendInvite(user: User) {
  resendingUserId.value = user.id;
  try {
    await usersStore.resendInvite(user.id);
    toast.add({
      severity: 'success',
      summary: 'Invite resent',
      detail: `Sent a fresh invite to ${user.email}.`,
      life: 3000,
    });
  } finally {
    resendingUserId.value = null;
  }
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text);
    return;
  } catch {
    // fallback to showing link
  }
  // FIXME: show link to copy as a fallback
}

function copiedLinkSummary(link: GeneratedUserAccessLink) {
  if (link.kind === 'password_reset') {
    return 'Password reset link copied';
  }
  return 'Invite link copied';
}

function copiedLinkDetail(link: GeneratedUserAccessLink, user: User) {
  if (link.kind === 'password_reset') {
    return `Fresh password reset link copied for ${user.email}.`;
  }
  return `Fresh invite link copied for ${user.email}.`;
}

async function copyAccessLink(user: User) {
  copyingLinkUserId.value = user.id;
  try {
    const link = await usersStore.generateAccessLink(user.id);
    await copyText(link.link);
    toast.add({
      severity: 'success',
      summary: copiedLinkSummary(link),
      detail: copiedLinkDetail(link, user),
      life: 3000,
    });
  } catch {
    toast.add({
      severity: 'error',
      summary: 'Copy failed',
      detail: usersStore.error ?? `Failed to generate a link for ${user.email}.`,
      life: 3000,
    });
  } finally {
    copyingLinkUserId.value = null;
  }
}

async function resendAllInvites() {
  resendingAllInvites.value = true;
  try {
    const resent = await usersStore.resendAllInvites();
    if (resent === 0) {
      toast.add({
        severity: 'info',
        summary: 'No pending invites',
        detail: 'There are no pending invites to resend.',
        life: 3000,
      });
      return;
    }
    toast.add({
      severity: 'success',
      summary: 'Invites resent',
      detail: `Resent ${resent} pending invite${resent === 1 ? '' : 's'}.`,
      life: 3000,
    });
  } finally {
    resendingAllInvites.value = false;
  }
}

function clearClientScope() {
  void router.push({ name: 'Users' });
}
</script>

<template>
  <div>
    <div class="flex items-center justify-between mb-4">
      <div>
        <h1 class="text-2xl font-bold">
          {{ scopedClient ? `Users for ${scopedClient.name}` : 'Users' }}
        </h1>
        <p class="text-sm text-muted-color">
          {{
            scopedClient
              ? 'Manage users assigned to this client.'
              : 'Manage admin, manager, and customer accounts.'
          }}
        </p>
      </div>
      <div class="flex items-center gap-2">
        <Button
          v-if="!scopedClientId"
          outlined
          severity="secondary"
          :disabled="resendingAllInvites || listingLoading || usersStore.loading"
          @click="resendAllInvites"
        >
          <span v-if="resendingAllInvites">Resending...</span>
          <span v-else>Resend All Invites</span>
        </Button>
        <Button v-if="scopedClientId" outlined severity="secondary" @click="clearClientScope">
          View All Users
        </Button>
        <Button :disabled="resendingAllInvites || usersStore.loading" @click="showCreate = true">
          New User
        </Button>
      </div>
    </div>

    <div class="flex flex-col gap-3 mb-4">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-end">
        <div class="w-full max-w-md">
          <label class="block text-sm font-medium text-muted-color mb-1">Search</label>
          <InputText
            v-model="search"
            type="text"
            fluid
            placeholder="Search users by name, email, role, or client..."
            class="px-3 py-2 border rounded"
          />
        </div>
        <div class="w-full lg:w-64">
          <label class="block text-sm font-medium text-muted-color mb-1">Invite Status</label>
          <Select
            v-model="selectedInviteStatus"
            :options="inviteStatusOptions"
            optionLabel="label"
            optionValue="value"
            class="w-full"
          />
        </div>
        <div class="w-full lg:w-56">
          <label class="block text-sm font-medium text-muted-color mb-1">Role</label>
          <Select
            v-model="selectedRole"
            :options="roleOptions"
            optionLabel="label"
            optionValue="value"
            class="w-full"
          />
        </div>
      </div>
      <div v-if="usersStore.error" class="text-red-600">{{ usersStore.error }}</div>
      <div v-if="clientsStore.error" class="text-red-600">{{ clientsStore.error }}</div>
    </div>

    <Card class="mt-4 shadow">
      <template #header>
        <div class="px-4 pt-3 border-b border-slate-200 flex items-center justify-between">
          <h2 class="font-semibold text-slate-900">User Directory</h2>
        </div>
      </template>
      <template #content>
        <div class="w-full overflow-x-auto">
          <table class="min-w-full rounded shadow">
            <thead>
              <tr>
                <th class="px-4 py-2 text-left">Name</th>
                <th class="px-4 py-2 text-left">Email</th>
                <th class="px-4 py-2 text-left">Role</th>
                <th class="px-4 py-2 text-left">Invite Status</th>
                <th class="px-4 py-2 text-left">Clients</th>
                <th class="px-4 py-2 text-left">Created</th>
                <th class="px-4 py-2 text-left">Actions</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="!listingLoading && listedUsers.length === 0">
                <td colspan="7" class="px-4 py-6 text-center text-gray-500">No users found.</td>
              </tr>
              <UserListItem
                v-for="user in listedUsers"
                :key="user.id"
                :user
                :disabled="
                  resendingAllInvites || usersStore.loading || copyingLinkUserId === user.id
                "
                @copy-link="copyAccessLink(user)"
                @edit="editUser(user)"
                @delete="confirmDelete(user)"
                @resend-invite="resendInvite(user)"
              />
            </tbody>
          </table>
        </div>
      </template>
      <template v-if="listingPagination.total_items > 0 && !listingLoading" #footer>
        <div class="flex items-center justify-between pt-3 border-t border-slate-200">
          <div class="text-sm text-slate-600">
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

    <UserModal
      :show="showCreate"
      mode="add"
      :clients="clientsStore.clients"
      :defaultRole="createUserDefaultRole"
      :defaultClientIds="createUserDefaultClientIds"
      :error="usersStore.error ?? undefined"
      :loading="usersStore.loading"
      @close="showCreate = false"
      @submit="createUser"
    />

    <UserModal
      :show="showEdit"
      mode="edit"
      :user="editUserObj"
      :clients="clientsStore.clients"
      :error="usersStore.error ?? undefined"
      :loading="usersStore.loading"
      @close="showEdit = false"
      @submit="updateUser"
    />

    <DeleteUserModal
      :show="showDelete"
      :userName="deleteUserObj?.name || deleteUserObj?.email || ''"
      :error="usersStore.error ?? undefined"
      @close="showDelete = false"
      @delete="removeUser"
    />
  </div>
</template>

<style scoped></style>
