<script setup lang="ts">
import { Button, Dialog, useToast } from 'primevue';

import {
  dismissPasskeyPrompt,
  passkeyErrorMessage,
  passkeysSupported,
  registerPasskey,
} from '@/services/passkeys';
import { useAuthStore } from '@/stores/auth';

const auth = useAuthStore();
const toast = useToast();
const saving = ref(false);
const error = ref('');
const supported = passkeysSupported();

const visible = computed({
  get: () => auth.passkeyEnrollmentPrompt,
  set: (value: boolean) => {
    if (!value) {
      auth.clearPasskeyEnrollmentPrompt();
    }
  },
});

async function addPasskey() {
  saving.value = true;
  error.value = '';
  try {
    await registerPasskey();
    auth.clearPasskeyEnrollmentPrompt();
    toast.add({ severity: 'success', summary: 'Passkey added', life: 3000 });
  } catch (err) {
    error.value = passkeyErrorMessage(
      err,
      'Unable to add a passkey. Try again or use Security later.',
    );
  } finally {
    saving.value = false;
  }
}

async function skipAndDismiss() {
  saving.value = true;
  error.value = '';
  try {
    await dismissPasskeyPrompt();
    auth.clearPasskeyEnrollmentPrompt();
  } catch (err) {
    error.value = passkeyErrorMessage(err, 'Unable to save your preference.');
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <Dialog v-model:visible="visible" modal :closable="!saving" :style="{ width: '30rem' }">
    <template #header>
      <div class="flex items-center gap-3">
        <i class="pi pi-key text-primary text-xl" />
        <span class="font-semibold">Add a passkey</span>
      </div>
    </template>

    <div class="space-y-3">
      <p>
        Sign in faster with your device's screen lock, fingerprint, or security key instead of your
        password.
      </p>
      <p v-if="!supported" class="text-sm text-orange-700">
        Passkeys need a secure connection and a compatible browser. You can add one later from
        Security.
      </p>
      <p v-if="error" class="text-sm text-red-600">{{ error }}</p>
    </div>

    <template #footer>
      <div class="flex flex-wrap justify-end gap-2">
        <Button
          label="Skip"
          severity="secondary"
          text
          :disabled="saving"
          @click="visible = false"
        />
        <Button
          label="Don't ask again"
          severity="secondary"
          text
          :disabled="saving"
          @click="skipAndDismiss"
        />
        <Button
          label="Add passkey"
          icon="pi pi-key"
          :disabled="saving || !supported"
          :loading="saving"
          @click="addPasskey"
        />
      </div>
    </template>
  </Dialog>
</template>
