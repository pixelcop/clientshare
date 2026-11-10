<script setup lang="ts">
import axios from 'axios';
import { Button, Dialog, InputText } from 'openvue';
import { storeToRefs } from 'pinia';

import { useAuthStore } from '@/stores/auth';
import type { QueuedEmail } from '@/types/models';
import { formatDate, formatDateNatural } from '@/types/util';

const { clientId } = defineProps<{ clientId: string }>();

const { user } = storeToRefs(useAuthStore());

const showPending = ref(false);

const canManageEmailQueue = computed(() => ['admin', 'manager'].includes(user.value?.role ?? ''));

const pendingEmails = ref<QueuedEmail[]>([]);
const pendingCount = computed(() => pendingEmails.value.length);
const pendingLoading = ref(false);
const pendingError = ref('');
const pendingSuccess = ref('');
let requestVersion = 0;

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
  requestVersion += 1;
  const version = requestVersion;
  pendingLoading.value = true;
  pendingError.value = '';
  pendingSuccess.value = '';
  try {
    const res = await axios.get('/api/email/queue', {
      params: { client_id: clientId },
    });
    if (version === requestVersion) {pendingEmails.value = res.data?.pending ?? [];}
  } catch (err) {
    if (version !== requestVersion) {return;}
    if (axios.isAxiosError(err)) {
      pendingError.value = err.response?.data?.error || 'Failed to load pending emails.';
    } else {
      pendingError.value = 'Failed to load pending emails.';
    }
  } finally {
    if (version === requestVersion) {pendingLoading.value = false;}
  }
}

const timeLeft = computed(() => {
  if (!pendingEmails.value?.[0]?.scheduled_for) {
    return '15 minutes';
  }
  const scheduledFor = new Date(pendingEmails.value[0].scheduled_for);
  return formatDateNatural(scheduledFor);
});

watch(
  () => clientId,
  () => {
    requestVersion += 1;
    pendingEmails.value = [];
    showPending.value = false;
    void fetchPendingEmails();
  },
  { immediate: true },
);

defineExpose({ fetchPendingEmails });
</script>

<template>
  <Button
    v-if="canManageEmailQueue"
    label="Pending email"
    icon="oi oi-envelope"
    severity="secondary"
    variant="text"
    :badge="pendingCount ? String(pendingCount) : undefined"
    @click="showPending = true"
  />

  <Dialog
    v-model:visible="showPending"
    modal
    closable
    dismissableMask
    blockScroll
    aria-label="Pending email for this client"
    class="shadow w-[calc(100%-2rem)] max-w-4xl"
  >
    <template #header>
      <div class="flex flex-row items-stretch w-full">
        <div class="p-dialog-title w-full">
          Pending email for this client
          <div v-if="pendingCount" class="text-sm text-gray-400">
            Will be sent in {{ timeLeft }} unless more files are added
          </div>
        </div>
        <Button
          :disabled="pendingLoading"
          @click="fetchPendingEmails"
          severity="secondary"
          icon="oi oi-refresh"
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
