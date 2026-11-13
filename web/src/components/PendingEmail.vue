<script setup lang="ts">
import axios from 'axios';
import { Button, Dialog, InputText, Popover, ToggleSwitch } from 'openvue';
import { storeToRefs } from 'pinia';

import { useAuthStore } from '@/stores/auth';
import type { QueuedEmail } from '@/types/models';
import { formatDate, formatDateNatural } from '@/types/util';

const {
  clientId,
  canManageQueue,
  canNotify = false,
  disabled = false,
  notificationsDisabled = false,
} = defineProps<{
  clientId: string;
  canManageQueue: boolean;
  canNotify?: boolean;
  disabled?: boolean;
  notificationsDisabled?: boolean;
}>();
const notificationsEnabled = defineModel<boolean>('notificationsEnabled', { default: true });
const emailOptions = ref<InstanceType<typeof Popover> | null>(null);
const notificationId = useId();
const emailTrigger = shallowRef<HTMLElement | null>(null);

function toggleEmailOptions(event: MouseEvent) {
  emailTrigger.value = event.currentTarget as HTMLElement;
  emailOptions.value?.toggle(event);
}

const { user } = storeToRefs(useAuthStore());

const showPending = ref(false);

const canManageEmailQueue = computed(
  () => canManageQueue && ['admin', 'manager'].includes(user.value?.role ?? ''),
);

function openPendingEmails() {
  emailOptions.value?.hide();
  showPending.value = true;
  void fetchPendingEmails();
}

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
    if (version === requestVersion) {
      pendingEmails.value = res.data?.pending ?? [];
    }
  } catch (err) {
    if (version !== requestVersion) {
      return;
    }
    if (axios.isAxiosError(err)) {
      pendingError.value = err.response?.data?.error || 'Failed to load pending emails.';
    } else {
      pendingError.value = 'Failed to load pending emails.';
    }
  } finally {
    if (version === requestVersion) {
      pendingLoading.value = false;
    }
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
  () => [clientId, canManageEmailQueue.value],
  () => {
    requestVersion += 1;
    pendingEmails.value = [];
    showPending.value = false;
    emailOptions.value?.hide();
    void fetchPendingEmails();
  },
  { immediate: true },
);

defineExpose({ fetchPendingEmails });
</script>

<template>
  <Button
    v-if="canManageEmailQueue || canNotify"
    type="button"
    icon="oi oi-envelope"
    severity="secondary"
    class="email-options-button"
    :aria-label="canManageEmailQueue ? `Email options, ${pendingCount} pending` : 'Email options'"
    :disabled
    :badge="pendingCount ? String(pendingCount) : undefined"
    @click="toggleEmailOptions"
  />

  <Popover ref="emailOptions" aria-label="Email options">
    <div class="email-options">
      <Button
        v-if="canManageEmailQueue"
        type="button"
        autofocus
        severity="secondary"
        variant="text"
        class="pending-email-action"
        @click="openPendingEmails"
      >
        <i class="oi oi-envelope" aria-hidden="true" />
        <span>Pending emails</span>
        <span class="pending-email-count">{{ pendingCount }}</span>
      </Button>
      <div v-if="canNotify" class="notification-option">
        <label :for="notificationId">Email notifications</label>
        <ToggleSwitch
          v-model="notificationsEnabled"
          :inputId="notificationId"
          :disabled="notificationsDisabled || disabled"
        />
      </div>
    </div>
  </Popover>

  <Dialog
    v-model:visible="showPending"
    modal
    closable
    dismissableMask
    blockScroll
    aria-label="Pending email for this client"
    class="shadow w-[calc(100%-2rem)] max-w-4xl"
    @after-hide="emailTrigger?.focus()"
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
      data-sentry-mask
      class="mt-3 rounded border border-red-200 bg-red-50 text-red-700 px-3 py-2 text-sm"
    >
      {{ pendingError }}
    </div>
    <div
      v-if="pendingSuccess"
      data-sentry-mask
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
.email-options-button {
  min-width: 44px;
  min-height: 44px;
  font-variant-numeric: tabular-nums;
}
.email-options {
  min-width: 240px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.pending-email-action,
.notification-option {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 44px;
  padding: 8px 12px;
  font-size: 14px;
}
.pending-email-action {
  justify-content: flex-start;
  width: 100%;
}
.pending-email-count {
  margin-left: auto;
  color: var(--p-text-muted-color);
  font-variant-numeric: tabular-nums;
}
.notification-option {
  justify-content: space-between;
}
.notification-option label {
  cursor: pointer;
}
</style>
