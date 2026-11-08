<script setup lang="ts">
import { Button, Dialog } from 'openvue';

import { useAuthStore } from '@/stores/auth';

defineProps<{ visible: boolean; clientId: string; clientName: string }>();
const emit = defineEmits<{ close: [] }>();
const auth = useAuthStore();
const canManagePeople = computed(() => ['admin', 'manager'].includes(auth.user?.role || ''));
const canManageLinks = computed(() => auth.user?.role === 'admin');
</script>

<template>
  <Dialog
    :visible="visible"
    :header="`Manage client access: ${clientName}`"
    :aria-label="`Manage client access: ${clientName}`"
    modal
    blockScroll
    position="right"
    :draggable="false"
    :style="{ width: '32rem', maxWidth: 'calc(100vw - 2rem)' }"
    @update:visible="(value) => !value && emit('close')"
  >
    <div class="client-access">
      <p class="access-scope">Scope: entire client workspace</p>
      <p class="access-note">
        These controls apply to {{ clientName }}, not just the current folder or file.
      </p>
      <section v-if="canManagePeople" aria-labelledby="client-access-people">
        <h3 id="client-access-people"><i class="oi oi-users" aria-hidden="true" /> People</h3>
        <p>
          Manage named accounts assigned to this client. Account access is separate from bearer
          links.
        </p>
        <Button asChild v-slot="slotProps" severity="secondary">
          <router-link
            :class="slotProps.class"
            :to="{ name: 'Users', query: { client_id: clientId } }"
            @click="emit('close')"
            >Manage people</router-link
          >
        </Button>
      </section>
      <section v-if="canManageLinks" aria-labelledby="client-access-links">
        <h3 id="client-access-links"><i class="oi oi-link" aria-hidden="true" /> Links</h3>
        <p>
          Anyone holding an active link can use its permitted actions until it expires or is
          revoked.
        </p>
        <Button asChild v-slot="slotProps" severity="secondary">
          <router-link
            :class="slotProps.class"
            :to="`/clients/${clientId}/links`"
            @click="emit('close')"
            >Manage links</router-link
          >
        </Button>
      </section>
      <p v-if="!canManagePeople && !canManageLinks">
        You do not have permission to manage this client’s access.
      </p>
    </div>
  </Dialog>
</template>

<style scoped>
.client-access {
  display: grid;
  gap: 16px;
  font-size: 14px;
}
.access-scope {
  font-weight: 600;
  color: var(--p-text-color);
}
.access-note,
.client-access section p {
  color: var(--p-text-muted-color);
  line-height: 1.5;
}
.client-access section {
  padding-top: 16px;
  border-top: 1px solid var(--p-content-border-color);
}
.client-access h3 {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 18px;
  font-weight: 600;
  margin: 0 0 8px;
}
.client-access section p {
  margin-bottom: 12px;
}
.client-access :deep(.p-button) {
  min-height: 44px;
}
</style>
