<script setup lang="ts">
import axios from 'axios';
import { storeToRefs } from 'pinia';
import { Button, FloatLabel, Password } from 'primevue';
import { computed, ref } from 'vue';
import { useRoute } from 'vue-router';

import Logo from '@/components/Logo.vue';
import { useBrandingStore } from '@/stores/branding';

const { branding } = storeToRefs(useBrandingStore());
const route = useRoute();

const token = computed(() => {
  const hash = route.hash.startsWith('#') ? route.hash.slice(1) : route.hash;
  return new URLSearchParams(hash).get('token') || '';
});

const password = ref('');
const confirmPassword = ref('');
const loading = ref(false);
const message = ref<string | null>(null);
const error = ref<string | null>(null);

const tokenMissing = computed(() => token.value === '');

function resolveErrorMessage(err: unknown, fallback: string) {
  if (axios.isAxiosError(err)) {
    return err.response?.data?.error || fallback;
  }
  if (err instanceof Error) {
    return err.message;
  }
  return fallback;
}

async function onSubmit() {
  if (password.value !== confirmPassword.value) {
    error.value = 'Passwords do not match.';
    return;
  }
  loading.value = true;
  error.value = null;
  message.value = null;
  try {
    await axios.post('/api/auth/accept-invite', {
      token: token.value,
      password: password.value,
    });
    message.value = 'Password set. Signing you in...';
    setTimeout(() => {
      window.location.href = '/';
    }, 400);
  } catch (e: unknown) {
    error.value = resolveErrorMessage(e, 'Unable to accept invite.');
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <div
    class="flex sm:h-full pt-6 sm:pt-0 @max-xs:hidden sm:bg-gray-50 px-4 sm:items-center justify-center"
  >
    <div class="w-full max-w-md p-5 sm:p-6 bg-white rounded shadow">
      <div class="flex flex-col items-center justify-center">
        <img v-if="branding.logo" :src="branding.logo" alt="Logo" class="max-w-1/2" />
        <Logo v-else class="text-7xl!" />
      </div>
      <h1 class="text-2xl font-bold mb-6 text-center">Set your password</h1>
      <div v-if="tokenMissing" class="text-red-600 text-center">
        Invalid or missing invite token.
      </div>
      <form v-else @submit.prevent="onSubmit">
        <div class="mb-4">
          <FloatLabel variant="on">
            <Password v-model="password" id="password" toggleMask fluid :feedback="false" focus />
            <label for="password">New password</label>
          </FloatLabel>
        </div>
        <div class="mb-6">
          <FloatLabel variant="on">
            <Password v-model="confirmPassword" id="confirm" toggleMask fluid :feedback="false" />
            <label for="confirm">Confirm password</label>
          </FloatLabel>
        </div>
        <Button type="submit" class="w-full" :disabled="loading">
          <span v-if="loading">Saving...</span>
          <span v-else>Set password</span>
        </Button>
        <div v-if="message" class="mt-4 text-green-700 text-center">{{ message }}</div>
        <div v-if="error" class="mt-4 text-red-600 text-center">{{ error }}</div>
      </form>
      <div class="mt-6 text-center">
        <router-link class="text-sm text-blue-600 hover:underline" to="/login"
          >Back to sign in</router-link
        >
      </div>
    </div>
  </div>
</template>
