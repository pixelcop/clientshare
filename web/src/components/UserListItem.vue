<script setup lang="ts">
import { breakpointsTailwind, useBreakpoints } from '@vueuse/core';
import type { MenuItem } from 'primevue/menuitem';

import { useClientsStore } from '@/stores/clients';
import type { Client, User } from '@/types/models';
import { formatDate } from '@/types/util';

const { user } = defineProps<{
  user: User;
  disabled?: boolean;
}>();

const emit = defineEmits<{
  (e: 'edit', user: User): void;
  (e: 'delete', user: User): void;
  (e: 'copy-link', user: User): void;
  (e: 'resend-invite', user: User): void;
}>();

const breakpoints = useBreakpoints(breakpointsTailwind);
const xs = breakpoints.smaller('sm');

const clientsStore = useClientsStore();

const clientsById = computed(() => {
  const map = new Map<string, Client>();
  clientsStore.clients.forEach((client) => map.set(client.id, client));
  return map;
});

function clientNames(user: User) {
  if (!user.client_ids || user.client_ids.length === 0) {
    return '-';
  }
  return user.client_ids
    .map((clientId) => clientsById.value.get(clientId)?.name || `Client #${clientId}`)
    .join(', ');
}

function inviteStatusLabel(user: User) {
  return user.invite_accepted ? 'Accepted' : 'Invite pending';
}

function inviteStatusClass(user: User) {
  if (user.invite_accepted) {
    return 'bg-emerald-50 text-emerald-700 ring-emerald-200';
  }
  return 'bg-amber-50 text-amber-700 ring-amber-200';
}

function copyLinkLabel(user: User) {
  if (user.invite_accepted) {
    return 'Copy Password Reset Link';
  }
  return 'Copy Invite Link';
}

const primaryLabel = computed(() => {
  if (xs.value) {
    return undefined;
  }
  if (user.invite_accepted) {
    return 'Edit';
  }
  return 'Resend Invite';
});

const primaryIcon = computed(() => {
  if (user.invite_accepted) {
    return 'pi pi-pencil';
  }
  return 'pi pi-send';
});

const primaryAction = () => {
  if (user.invite_accepted) {
    emit('edit', user);
  } else {
    emit('resend-invite', user);
  }
};

const userActions = computed<MenuItem[]>(() => {
  const actions: MenuItem[] = [];

  if (!user.invite_accepted) {
    actions.push({
      label: 'Resend Invite',
      icon: 'pi pi-send',
      command: () => emit('resend-invite', user),
    });
  }
  actions.push(
    {
      label: copyLinkLabel(user),
      icon: 'pi pi-copy',
      command: () => emit('copy-link', user),
    },
    {
      label: 'Edit',
      icon: 'pi pi-pencil',
      command: () => emit('edit', user),
    },
    {
      label: 'Delete',
      icon: 'pi pi-trash',
      command: () => emit('delete', user),
    },
  );

  return actions;
});
</script>

<template>
  <tr>
    <td class="px-4 py-2">{{ user.name || '-' }}</td>
    <td class="px-4 py-2">{{ user.email }}</td>
    <td class="px-4 py-2 capitalize">{{ user.role }}</td>
    <td class="px-4 py-2">
      <span
        class="inline-flex items-center rounded-full px-2.5 py-1 text-xs font-medium ring-1 ring-inset text-nowrap"
        :class="inviteStatusClass(user)"
      >
        {{ inviteStatusLabel(user) }}
      </span>
    </td>
    <td class="px-4 py-2">
      <span v-if="user.role === 'customer' || user.role === 'client'">{{ clientNames(user) }}</span>
      <span v-else class="text-gray-400">—</span>
    </td>
    <td class="px-4 py-2">{{ formatDate(user.created_at) }}</td>
    <td class="px-4 py-2">
      <div class="flex gap-2 flex-wrap">
        <SplitButton
          :label="primaryLabel"
          :icon="primaryIcon"
          @click.stop="primaryAction"
          :model="userActions"
          severity="info"
          :disabled
        />
      </div>
    </td>
  </tr>
</template>
