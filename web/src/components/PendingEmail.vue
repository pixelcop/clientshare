<script setup lang="ts">
import axios from 'axios';
import { formatDistance } from 'date-fns';
import { storeToRefs } from 'pinia';
import { Button, Dialog, InputText } from 'primevue';

import { useAuthStore } from '@/stores/auth';
import type { QueuedEmail } from '@/types/models';
import { formatDate } from '@/types/util';

const { clientId } = defineProps<{ clientId: string }>();

const { user } = storeToRefs(useAuthStore());

// button effects
const justLeft = ref(false);
const showButton = ref(false);
function onHoverBadge() {
  if (justLeft.value) {
    return;
  }
  showButton.value = true;
}
function onMouseLeave() {
  justLeft.value = true;
  showButton.value = false;
  setTimeout(() => {
    justLeft.value = false;
  }, 300);
}
// end button effects

const showPending = ref(false);
const pendingCount = computed(() => {
  if (!pendingEmails.value?.length) {
    return 0;
  }
  return Array.from(pendingEmails.value[0]?.body.matchAll(/A new file/g) || []).length;
});

const canManageEmailQueue = computed(() => ['admin', 'manager'].includes(user.value?.role ?? ''));

const pendingEmails = ref<QueuedEmail[]>([]);
const pendingLoading = ref(false);
const pendingError = ref('');
const pendingSuccess = ref('');

const sendingEmailIds = ref<string[]>([]);

function isSendingEmail(id: string) {
  return sendingEmailIds.value.includes(id);
}

function setSendingEmail(id: string, sending: boolean) {
  if (sending) {
    if (!sendingEmailIds.value.includes(id)) {
      sendingEmailIds.value = [...sendingEmailIds.value, id];
    }
    return;
  }
  sendingEmailIds.value = sendingEmailIds.value.filter((value) => value !== id);
}

async function sendEmailNow(email: QueuedEmail) {
  pendingError.value = '';
  pendingSuccess.value = '';
  setSendingEmail(email.id, true);
  try {
    await axios.post(`/api/email/queue/${email.id}/send`, {
      recipient: email.recipient.trim(),
      subject: email.subject.trim(),
      body: email.body,
    });
    pendingSuccess.value = 'Email sent.';
    await fetchPendingEmails();
  } catch (err) {
    if (axios.isAxiosError(err)) {
      pendingError.value = err.response?.data?.error || 'Failed to send email.';
    } else {
      pendingError.value = 'Failed to send email.';
    }
  } finally {
    setSendingEmail(email.id, false);
    showPending.value = false;
  }
}

async function fetchPendingEmails() {
  if (!canManageEmailQueue.value || !clientId) {
    return;
  }
  pendingLoading.value = true;
  pendingError.value = '';
  pendingSuccess.value = '';
  try {
    const res = await axios.get('/api/email/queue', {
      params: { client_id: clientId },
    });
    pendingEmails.value = res.data?.pending ?? [];
  } catch (err) {
    if (axios.isAxiosError(err)) {
      pendingError.value = err.response?.data?.error || 'Failed to load pending emails.';
    } else {
      pendingError.value = 'Failed to load pending emails.';
    }
  } finally {
    pendingLoading.value = false;
  }
}

const timeLeft = computed(() => {
  if (!pendingEmails.value?.[0]?.scheduled_for) {
    return '15 minutes';
  }
  const scheduledFor = new Date(pendingEmails.value[0].scheduled_for);
  return formatDistance(scheduledFor, new Date(), { addSuffix: true });
});

onMounted(() => {
  void fetchPendingEmails();
});

defineExpose({ fetchPendingEmails });
</script>

<template>
  <i
    v-if="pendingCount === 0"
    class="pi pi-envelope text-2xl no-emails"
    v-tooltip.left="'No pending email'"
  />
  <div v-else class="email-badge-container" @click="showPending = true">
    <Transition name="badge-fade">
      <OverlayBadge
        v-if="!showButton"
        :value="pendingCount"
        size="small"
        class="badge-icon"
        @mouseenter="onHoverBadge"
      >
        <i class="pi pi-envelope" style="font-size: 2rem" />
      </OverlayBadge>
    </Transition>
    <Transition name="button-fade">
      <Button
        v-if="showButton"
        type="button"
        label="View pending email"
        icon="pi pi-envelope"
        :badge="pendingCount.toString()"
        size="large"
        class="inbox-button"
        @mouseleave="onMouseLeave"
      />
    </Transition>
  </div>

  <Dialog
    v-model:visible="showPending"
    modal
    closable
    dismissableMask
    blockScroll
    class="shadow w-3/4"
  >
    <template #header>
      <div class="flex flex-row items-stretch w-full">
        <div class="p-dialog-title w-full">
          Pending email for this client
          <div class="text-sm text-gray-400">
            Will be sent in {{ timeLeft }} unless more files are added
          </div>
        </div>
        <Button
          :disabled="pendingLoading"
          @click="fetchPendingEmails"
          severity="secondary"
          icon="pi pi-refresh"
          label="Refresh"
        />
      </div>
    </template>
    <div
      v-if="pendingError"
      class="mt-3 rounded border border-red-200 bg-red-50 text-red-700 px-3 py-2 text-sm"
    >
      {{ pendingError }}
    </div>
    <div
      v-if="pendingSuccess"
      class="mt-3 rounded border border-green-200 bg-green-50 text-green-700 px-3 py-2 text-sm"
    >
      {{ pendingSuccess }}
    </div>
    <div v-if="pendingLoading" class="mt-3 text-sm text-gray-500">Loading pending emails...</div>
    <div v-else class="mt-3">
      <div v-if="pendingEmails.length === 0" class="text-sm">
        No pending emails for this client.
      </div>
      <div v-else class="space-y-4 flex flex-col">
        <div
          v-for="email in pendingEmails"
          :key="email.id"
          class="border rounded p-4 flex flex-col"
        >
          <div class="grid gap-3 md:grid-cols-2">
            <div>
              <label class="text-xs font-semibold text-gray-600">Recipient</label>
              <InputText v-model="email.recipient" type="email" fluid class="mt-1" size="small" />
            </div>
            <div>
              <label class="text-xs font-semibold text-gray-600">Subject</label>
              <InputText v-model="email.subject" type="text" fluid class="mt-1" size="small" />
            </div>
          </div>
          <div class="mt-3">
            <label class="text-xs font-semibold text-gray-600">Body</label>
            <iframe class="mt-2 w-full min-h-96" :srcdoc="email.html_body"></iframe>
          </div>
          <div class="mt-3 flex flex-wrap items-center justify-between gap-3 text-xs text-gray-500">
            <div class="flex flex-wrap gap-4">
              <span>Scheduled: {{ formatDate(email.scheduled_for) }}</span>
              <span>Queued: {{ formatDate(email.created_at) }}</span>
            </div>
            <Button
              :disabled="isSendingEmail(email.id)"
              @click="sendEmailNow(email)"
              severity="info"
              size="small"
            >
              {{ isSendingEmail(email.id) ? 'Sending...' : 'Send now' }}
            </Button>
          </div>
        </div>
      </div>
    </div>
  </Dialog>
</template>

<style scoped>
.no-emails {
  color: var(--p-form-field-disabled-color);
  opacity: var(--p-disabled-opacity);
}
.email-badge-container {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  min-width: 120px;
  min-height: 48px;
}

.badge-icon {
  position: absolute;
  right: 0;
  opacity: 1;
  transform: translateX(0) scale(1);
  transition:
    opacity 0.5s ease,
    transform 0.5s ease;
  cursor: pointer;
}

.inbox-button {
  transform-origin: right center;
  transition:
    opacity 0.5s ease,
    transform 0.5s ease;
}

.badge-fade-leave-active {
  transform: translateX(-190px) scale(0.6);
}

.badge-fade-enter-from,
.badge-fade-leave-to {
  opacity: 0;
}

.button-fade-enter-active,
.button-fade-leave-active {
  transition:
    opacity 0.45s ease,
    transform 0.45s ease;
}

.button-fade-enter-from,
.button-fade-leave-to {
  opacity: 0;
  transform: scaleX(0);
}

.button-fade-enter-to,
.button-fade-leave-from {
  opacity: 1;
  transform: scaleX(1);
}
</style>
