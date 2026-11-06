<script setup lang="ts">
import StaffSeatUsage from '@/components/StaffSeatUsage.vue';
import { useStaffQuotaStore } from '@/stores/staffQuota';
import type { Client, User } from '@/types/models';

const props = defineProps<{
  show: boolean;
  mode: 'add' | 'edit';
  user?: User | null;
  clients: Client[];
  defaultRole?: 'admin' | 'manager' | 'client';
  defaultClientIds?: string[];
  error?: string;
  loading?: boolean;
}>();

const emit = defineEmits(['submit', 'close']);

const name = ref('');
const email = ref('');
const role = ref<'admin' | 'manager' | 'client'>('manager');
const clientIds = ref<string[]>([]);
const localError = ref<string | null>(null);

const isEdit = computed(() => props.mode === 'edit');
const quota = useStaffQuotaStore();
const existingStaff = computed(
  () => isEdit.value && (props.user?.role === 'admin' || props.user?.role === 'manager'),
);
const staffBlocked = computed(() => !existingStaff.value && !quota.canAddStaff);
const invitationBlocked = computed(() => role.value !== 'client' && staffBlocked.value);

// Billing opens in another tab. Recheck seats on return without resetting the draft.
function refreshQuotaOnFocus() {
  if (props.show) {
    void quota.refresh();
  }
}
onMounted(() => window.addEventListener('focus', refreshQuotaOnFocus));
onBeforeUnmount(() => window.removeEventListener('focus', refreshQuotaOnFocus));

function resetForm() {
  name.value = props.user?.name || '';
  email.value = props.user?.email || '';
  role.value =
    (props.user?.role as 'admin' | 'manager' | 'client') || props.defaultRole || 'manager';
  clientIds.value = props.user?.client_ids || props.defaultClientIds || [];
  localError.value = null;
}

watch(
  () => props.show,
  (show) => {
    if (show) {
      resetForm();
      void quota.refresh();
    }
  },
);

watch(
  () => props.user,
  () => {
    if (props.show) {
      resetForm();
    }
  },
);

function onSubmit() {
  localError.value = null;
  if (invitationBlocked.value || props.loading) {return;}
  if (!name.value.trim() || !email.value.trim()) {
    localError.value = 'Name and email are required.';
    return;
  }
  if (role.value === 'client' && clientIds.value.length === 0) {
    localError.value = 'Please assign at least one client for customer users.';
    return;
  }

  const payload: Record<string, unknown> = {
    name: name.value.trim(),
    email: email.value.trim(),
    role: role.value,
  };

  if (role.value === 'client') {
    payload.client_ids = clientIds.value;
  }

  emit('submit', payload);
}
</script>

<template>
  <Dialog
    :visible="show"
    @update:visible="
      (val) => {
        if (!val) {
          $emit('close');
        }
      }
    "
    modal
    closable
    dismissableMask
    blockScroll
    class="xl:min-w-2/5 md:min-w-3/5 sm:min-w-4/5 min-w-5/6"
  >
    <template #header>
      <div class="flex flex-col items-start">
        <h2 class="text-lg font-bold mb-0!">{{ isEdit ? 'Edit User' : 'New User' }}</h2>
        <p v-if="!isEdit" class="text-sm text-gray-600 mb-4">
          An invite email will be sent right away so this user can set their password.
        </p>
      </div>
    </template>
    <form @submit.prevent="onSubmit">
      <StaffSeatUsage v-if="!existingStaff" />
      <div class="grid grid-cols-1 gap-4">
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Role</label>
          <select v-model="role" class="px-3 py-2 border rounded w-full" :disabled="loading">
            <option value="admin" :disabled="staffBlocked">Admin</option>
            <option value="manager" :disabled="staffBlocked">Manager</option>
            <option value="client">Customer</option>
          </select>
        </div>
        <div v-if="!invitationBlocked">
          <label class="block text-sm font-medium text-gray-700 mb-1">Name</label>
          <input
            v-model="name"
            type="text"
            class="px-3 py-2 border rounded w-full"
            placeholder="Full name"
            required
            autofocus
            :disabled="loading"
          />
        </div>
        <div v-if="!invitationBlocked">
          <label class="block text-sm font-medium text-gray-700 mb-1">Email</label>
          <input
            v-model="email"
            type="email"
            class="px-3 py-2 border rounded w-full"
            placeholder="name@example.com"
            required
            :disabled="loading"
          />
        </div>
        <div v-if="role === 'client'">
          <label class="block text-sm font-medium text-gray-700 mb-1">Assign Clients</label>
          <MultiSelect
            v-model="clientIds"
            :options="clients"
            optionLabel="name"
            optionValue="id"
            placeholder="Select one or more clients"
            :disabled="loading"
            class="w-full"
            fluid
            filter
            autoFilterFocus
            display="chip"
          />
          <p v-if="clients.length === 0" class="text-xs text-gray-500 mt-1">
            No clients available. Create a client first.
          </p>
        </div>
      </div>
      <div class="flex justify-end gap-2 mt-6">
        <Button type="button" :disabled="loading" @click="$emit('close')" severity="secondary">
          Cancel
        </Button>
        <Button type="submit" :disabled="loading || invitationBlocked">
          {{ isEdit ? 'Save' : 'Create' }}
        </Button>
      </div>
    </form>
    <div v-if="localError" class="mt-2 text-red-600">{{ localError }}</div>
    <div v-else-if="error" class="mt-2 text-red-600">{{ error }}</div>
  </Dialog>
</template>

<style scoped></style>
