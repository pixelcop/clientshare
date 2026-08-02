<script setup lang="ts">
import { Button, Dialog, InputText, useConfirm, useToast } from 'primevue';

import {
  deletePasskey,
  listPasskeys,
  passkeyErrorMessage,
  passkeysSupported,
  registerPasskey,
  renamePasskey,
  type Passkey,
} from '@/services/passkeys';

const toast = useToast();
const confirm = useConfirm();
const passkeys = ref<Passkey[]>([]);
const loading = ref(true);
const saving = ref(false);
const error = ref('');
const renameVisible = ref(false);
const selectedPasskey = ref<Passkey | null>(null);
const newName = ref('');
const supported = passkeysSupported();

function formatDate(value?: string): string {
  if (!value) {
    return 'Never used';
  }
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' }).format(new Date(value));
}

async function loadPasskeys() {
  loading.value = true;
  error.value = '';
  try {
    passkeys.value = await listPasskeys();
  } catch (err) {
    error.value = passkeyErrorMessage(err, 'Unable to load passkeys.');
  } finally {
    loading.value = false;
  }
}

async function addPasskey() {
  saving.value = true;
  error.value = '';
  try {
    const passkey = await registerPasskey();
    passkeys.value.push(passkey);
    toast.add({ severity: 'success', summary: 'Passkey added', life: 3000 });
  } catch (err) {
    error.value = passkeyErrorMessage(err, 'Unable to add a passkey.');
  } finally {
    saving.value = false;
  }
}

function openRename(passkey: Passkey) {
  selectedPasskey.value = passkey;
  newName.value = passkey.name;
  renameVisible.value = true;
}

async function saveName() {
  if (!selectedPasskey.value || !newName.value.trim()) {
    return;
  }
  saving.value = true;
  try {
    const updated = await renamePasskey(selectedPasskey.value.id, newName.value.trim());
    const index = passkeys.value.findIndex((passkey) => passkey.id === updated.id);
    if (index >= 0) {
      passkeys.value[index] = updated;
    }
    renameVisible.value = false;
  } catch (err) {
    error.value = passkeyErrorMessage(err, 'Unable to rename passkey.');
  } finally {
    saving.value = false;
  }
}

function removePasskey(passkey: Passkey) {
  confirm.require({
    message: `Remove ${passkey.name}? You will no longer be able to use it to sign in.`,
    header: 'Remove passkey',
    icon: 'pi pi-exclamation-triangle',
    acceptClass: 'p-button-danger',
    accept: async () => {
      try {
        await deletePasskey(passkey.id);
        passkeys.value = passkeys.value.filter((item) => item.id !== passkey.id);
        toast.add({ severity: 'success', summary: 'Passkey removed', life: 3000 });
      } catch (err) {
        error.value = passkeyErrorMessage(err, 'Unable to remove passkey.');
      }
    },
  });
}

onMounted(() => {
  void loadPasskeys();
});
</script>

<template>
  <div class="max-w-3xl space-y-6">
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold">Security</h1>
        <p class="text-sm text-muted-color">Manage how you sign in to your account.</p>
      </div>
      <Button
        label="Add passkey"
        icon="pi pi-key"
        :disabled="saving || !supported"
        :loading="saving"
        @click="addPasskey"
      />
    </div>

    <Card>
      <template #title>Passkeys</template>
      <template #subtitle
        >Use your device's screen lock, fingerprint, or a security key to sign in.</template
      >
      <template #content>
        <p v-if="!supported" class="mb-4 text-sm text-orange-700">
          Passkeys require a secure connection and a compatible browser.
        </p>
        <p v-if="error" class="mb-4 text-sm text-red-600">{{ error }}</p>
        <div v-if="loading" class="text-sm text-muted-color">Loading passkeys...</div>
        <div
          v-else-if="passkeys.length === 0"
          class="rounded border border-dashed p-5 text-sm text-muted-color"
        >
          No passkeys yet. Add one to sign in without your password.
        </div>
        <div v-else class="divide-y rounded border">
          <div
            v-for="passkey in passkeys"
            :key="passkey.id"
            class="flex flex-wrap items-center gap-4 p-4"
          >
            <i class="pi pi-key text-primary" />
            <div class="min-w-40 flex-1">
              <p class="font-medium">{{ passkey.name }}</p>
              <p class="text-sm text-muted-color">
                Added {{ formatDate(passkey.created_at) }}. Last used
                {{ formatDate(passkey.last_used_at) }}.
              </p>
            </div>
            <div class="flex gap-2">
              <Button label="Rename" severity="secondary" text @click="openRename(passkey)" />
              <Button label="Remove" severity="danger" text @click="removePasskey(passkey)" />
            </div>
          </div>
        </div>
      </template>
    </Card>

    <Card>
      <template #title>Password</template>
      <template #content>
        <p class="text-sm text-muted-color">
          Your password remains available as a backup sign-in method. Use the password reset link
          from sign in if needed.
        </p>
      </template>
    </Card>

    <Dialog
      v-model:visible="renameVisible"
      modal
      header="Rename passkey"
      :style="{ width: '28rem' }"
    >
      <div class="space-y-2">
        <label for="passkey-name" class="font-medium">Name</label>
        <InputText id="passkey-name" v-model="newName" class="w-full" maxlength="100" autofocus />
      </div>
      <template #footer>
        <Button
          label="Cancel"
          severity="secondary"
          text
          :disabled="saving"
          @click="renameVisible = false"
        />
        <Button
          label="Save"
          :disabled="saving || !newName.trim()"
          :loading="saving"
          @click="saveName"
        />
      </template>
    </Dialog>
  </div>
</template>
