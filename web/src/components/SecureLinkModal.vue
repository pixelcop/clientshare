<script setup lang="ts">
import { Button, Dialog, InputText, Select } from 'primevue';

const props = defineProps<{
  show?: boolean;
  embedded?: boolean;
  loading?: boolean;
  error?: string | null;
  defaultExpiryDays?: number;
  defaultAccessType?: 'read' | 'write' | 'readwrite';
  defaultSendEmail?: boolean;
}>();

const emit = defineEmits<{
  (
    e: 'submit',
    payload: {
      accessType: string;
      expiryDays: number;
      email: string;
      sendEmail: boolean;
    },
  ): void;
  (e: 'close'): void;
}>();

const accessType = ref<'read' | 'write' | 'readwrite'>(props.defaultAccessType || 'readwrite');
const expiryDays = ref<number | string>(props.defaultExpiryDays || 90);
const sendEmail = ref(props.defaultSendEmail ?? true);
const email = ref('');
const maxExpiryDays = 120;
const expiryOptions = [7, 14, 30, 60, 90, 120];
const settingsStorageKey = 'clientshare:secure-link-settings';

const accessOptions = [
  { label: 'Read', value: 'read' },
  { label: 'Write', value: 'write' },
  { label: 'Read & Write', value: 'readwrite' },
];

function normalizeExpiry(input: number | string) {
  const parsed = Number(input);
  if (!Number.isFinite(parsed)) {
    return maxExpiryDays;
  }
  return Math.min(Math.max(Math.round(parsed), 1), maxExpiryDays);
}

function loadStoredSettings() {
  const defaults = {
    accessType: props.defaultAccessType || 'readwrite',
    expiryDays: Math.min(props.defaultExpiryDays || 90, maxExpiryDays),
    sendEmail: props.defaultSendEmail ?? true,
  };
  try {
    const raw = localStorage.getItem(settingsStorageKey);
    if (!raw) {
      return defaults;
    }
    const parsed = JSON.parse(raw) as Partial<typeof defaults>;
    const access = parsed.accessType;
    const validAccess = access === 'read' || access === 'write' || access === 'readwrite';
    return {
      accessType: validAccess ? access : defaults.accessType,
      expiryDays: Number.isFinite(Number(parsed.expiryDays))
        ? normalizeExpiry(parsed.expiryDays as number)
        : defaults.expiryDays,
      sendEmail: parsed.sendEmail ?? defaults.sendEmail,
    };
  } catch {
    return defaults;
  }
}

function persistStoredSettings() {
  const payload = {
    accessType: accessType.value,
    expiryDays: normalizeExpiry(expiryDays.value),
    sendEmail: sendEmail.value,
  };
  try {
    localStorage.setItem(settingsStorageKey, JSON.stringify(payload));
  } catch {
    // ignore localStorage failures
  }
}

function resetSettings() {
  const settings = loadStoredSettings();
  accessType.value = settings.accessType;
  expiryDays.value = settings.expiryDays;
  sendEmail.value = settings.sendEmail;
  email.value = '';
}

function getSettings() {
  const normalized = normalizeExpiry(expiryDays.value);
  expiryDays.value = normalized;
  return {
    accessType: accessType.value,
    expiryDays: normalized,
    sendEmail: sendEmail.value,
    email: sendEmail.value ? email.value.trim() : '',
  };
}

defineExpose({ getSettings, resetSettings });

watch(
  () => props.show,
  (val) => {
    if (props.embedded) {
      resetSettings();
      return;
    }
    if (val) {
      resetSettings();
    }
  },
  { immediate: true },
);

watch(
  expiryDays,
  (val) => {
    const parsed = Number(val);
    if (!Number.isFinite(parsed)) {
      return;
    }
    if (parsed > maxExpiryDays) {
      expiryDays.value = maxExpiryDays;
    }
    if (parsed < 1) {
      expiryDays.value = 1;
    }
  },
  { flush: 'sync' },
);

watch([accessType, expiryDays, sendEmail], persistStoredSettings);

function onVisibleChange(val: boolean) {
  if (!val) {
    emit('close');
  }
}

const expirySelect = ref();
function openExpiryDropdown() {
  expirySelect.value?.show?.();
}

function onSubmit() {
  const settings = getSettings();
  emit('submit', settings);
}

const dialogComponent = computed(() => (props.embedded ? 'div' : Dialog));
</script>

<template>
  <component
    :is="dialogComponent"
    :visible="!!show"
    @update:visible="onVisibleChange"
    modal
    header="New Secure Link"
    dismissableMask
    blockScroll
    :style="{ width: embedded ? '' : '32rem' }"
  >
    <form @submit.prevent="onSubmit">
      <label class="block text-sm font-medium text-gray-700 mb-1">Access Type</label>
      <Select
        v-model="accessType"
        :options="accessOptions"
        option-label="label"
        option-value="value"
        class="mb-4 px-3 py-2 border rounded w-full"
      />

      <label class="block text-sm font-medium text-gray-700 mb-1">Expiry (days)</label>
      <Select
        v-model="expiryDays"
        :options="expiryOptions"
        editable
        fluid
        placeholder="Select or type days"
        class="mb-2"
        ref="expirySelect"
        @focus="openExpiryDropdown"
        @click="openExpiryDropdown"
      />
      <p class="text-xs text-gray-500 mb-4">Max {{ maxExpiryDays }} days.</p>

      <label class="flex items-center gap-2 text-sm mb-2">
        <input v-model="sendEmail" type="checkbox" class="h-4 w-4" />
        Send link by email
      </label>
      <label class="block text-sm font-medium text-gray-700 mb-1">
        Notification Email (optional)
      </label>
      <InputText
        v-model="email"
        type="email"
        autofocus
        autocomplete="email"
        placeholder="person@example.com"
        class="mb-4 w-full"
        :disabled="!sendEmail"
        :required="sendEmail"
      />

      <div v-if="!embedded" class="flex justify-end gap-2">
        <Button type="button" @click="$emit('close')" severity="secondary">Cancel</Button>
        <Button type="submit" :disabled="loading">
          {{ loading ? 'Creating...' : 'Create Link' }}
        </Button>
      </div>
    </form>
    <div v-if="error" class="mt-2 text-red-600">{{ error }}</div>
  </component>
</template>

<style scoped></style>
