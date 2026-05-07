<script setup lang="ts">
import axios from 'axios';
import { storeToRefs } from 'pinia';
import { Button, FloatLabel, InputText } from 'primevue';
import { ref } from 'vue';

import Logo from '@/components/Logo.vue';
import { useBrandingStore } from '@/stores/branding';

const { branding } = storeToRefs(useBrandingStore());
const email = ref('');
const loading = ref(false);
const message = ref<string | null>(null);
const error = ref<string | null>(null);

async function onSubmit() {
  loading.value = true;
  error.value = null;
  message.value = null;
  try {
    await axios.post('/api/auth/forgot-password', { email: email.value });
    message.value = 'If an account exists with this email, a reset link has been sent.';
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Unable to request password reset.';
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
        <Logo v-else class="!text-7xl" />
      </div>
      <h1 class="text-2xl font-bold mb-6 text-center">Reset Password</h1>
      <p class="text-sm text-gray-600 mb-8! text-center">
        Enter your email and we will send you a reset link.
      </p>
      <form @submit.prevent="onSubmit">
        <div class="mb-6">
          <FloatLabel>
            <label class="block mb-1 font-medium" for="email">Email</label>
            <InputText
              v-model="email"
              id="email"
              type="email"
              required
              autofocus
              class="w-full px-3 py-2 border rounded"
            />
          </FloatLabel>
        </div>
        <Button type="submit" class="w-full" :disabled="loading">
          <span v-if="loading">Sending...</span>
          <span v-else>Send reset link</span>
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

<style scoped></style>
