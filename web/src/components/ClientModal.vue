<script setup lang="ts">
import { Button, Dialog, InputText } from 'primevue';

import SecureLinkModal from '@/components/SecureLinkModal.vue';

type LinkSettingsPayload = {
  name: string;
  generateLink: boolean;
  sendEmail: boolean;
  accessType: 'read' | 'write' | 'readwrite';
  expiryDays: number;
  email: string;
};

const props = defineProps<{
  show: boolean;
  mode: 'add' | 'edit';
  initialName?: string;
  error?: string;
  linkError?: string | null;
  defaultSecureLinkExpiryDays?: number;
}>();
const emit = defineEmits<{
  (e: 'submit', payload: LinkSettingsPayload | { name: string }): void;
  (e: 'close'): void;
}>();

const name = ref(props.initialName || '');
const generateLink = ref(false);
const isAddMode = computed(() => props.mode === 'add');
const linkSettingsRef = ref<InstanceType<typeof SecureLinkModal> | null>(null);
const linkSettingsKey = 'clientshare:new-client-generate-link';

watch(
  () => props.initialName,
  (val) => {
    name.value = val || '';
  },
);

function loadLinkSettings() {
  const defaults = {
    generateLink: false,
  };
  try {
    const raw = localStorage.getItem(linkSettingsKey);
    if (!raw) {
      return defaults;
    }
    const parsed = JSON.parse(raw) as Partial<typeof defaults>;
    return {
      generateLink: Boolean(parsed.generateLink),
    };
  } catch {
    return defaults;
  }
}

function persistLinkSettings() {
  if (!isAddMode.value) {
    return;
  }
  const payload = {
    generateLink: generateLink.value,
  };
  try {
    localStorage.setItem(linkSettingsKey, JSON.stringify(payload));
  } catch {
    // ignore localStorage failures
  }
}

watch(
  () => props.show,
  (val) => {
    if (val && isAddMode.value) {
      name.value = '';
      const stored = loadLinkSettings();
      generateLink.value = stored.generateLink;
      linkSettingsRef.value?.resetSettings?.();
    }
  },
);

watch(generateLink, persistLinkSettings);

const canSubmit = computed(() => {
  if (!name.value.trim()) {
    return false;
  }
  return true;
});

function onSubmit() {
  if (!isAddMode.value) {
    emit('submit', { name: name.value.trim() });
    return;
  }
  const settings = linkSettingsRef.value?.getSettings?.();
  emit('submit', {
    name: name.value.trim(),
    generateLink: generateLink.value,
    sendEmail: settings?.sendEmail ?? true,
    accessType: settings?.accessType ?? 'readwrite',
    expiryDays: settings?.expiryDays ?? props.defaultSecureLinkExpiryDays ?? 90,
    email: settings?.email ?? '',
  });
}
</script>

<template>
  <Dialog
    :visible="show"
    class="xl:min-w-2/5 md:min-w-3/5 sm:min-w-4/5 min-w-5/6"
    modal
    dismissable-mask
    closable
    @update:visible="$emit('close')"
  >
    <template #header>
      <h2 class="mb-0!">{{ mode === 'edit' ? 'Edit Client' : 'New Client' }}</h2>
    </template>
    <form @submit.prevent="onSubmit">
      <InputText
        autofocus
        v-model="name"
        type="text"
        placeholder="Client name"
        class="mb-4 px-3 py-2 border rounded w-full"
        required
      />
      <div v-if="mode === 'add'" class="mb-4 space-y-2">
        <label class="flex items-center gap-2 text-sm">
          <input v-model="generateLink" type="checkbox" class="h-4 w-4" />
          Generate secure link
        </label>
        <p class="text-xs text-gray-500">
          Create a shareable secure link after the client is created.
        </p>
      </div>
      <div v-if="mode === 'add' && generateLink" class="mb-4 rounded border bg-slate-50 p-3">
        <h3 class="text-sm font-semibold text-slate-700 mb-3">Secure link settings</h3>
        <SecureLinkModal
          ref="linkSettingsRef"
          show
          embedded
          :defaultExpiryDays="defaultSecureLinkExpiryDays"
        />
      </div>
      <div class="flex justify-end gap-2">
        <Button type="button" @click="$emit('close')" severity="secondary"> Cancel </Button>
        <Button type="submit" :disabled="!canSubmit">
          {{ mode === 'edit' ? 'Save' : 'Create' }}
        </Button>
      </div>
    </form>
    <div v-if="error" class="mt-2 text-red-600">{{ error }}</div>
    <div v-if="linkError" class="mt-2 text-red-600">{{ linkError }}</div>
  </Dialog>
</template>

<style scoped></style>
