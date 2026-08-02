<script setup lang="ts">
import { storeToRefs } from 'pinia';
import { Button, FloatLabel, InputText } from 'primevue';
import { onMounted } from 'vue';

import Logo from '@/components/Logo.vue';
import {
  beginPasskeySignIn,
  getPasskeyAssertion,
  passkeyErrorMessage,
  passkeysSupported,
} from '@/services/passkeys';
import { useBrandingStore } from '@/stores/branding';

import { useAuthStore } from '../stores/auth';

const { branding } = storeToRefs(useBrandingStore());

const email = ref('');
const password = ref('');

const router = useRouter();
const auth = useAuthStore();
const { user } = storeToRefs(auth);

const loading = ref(false);
const error = ref<string | null>(null);
const showPassword = ref(false);

function redirectAfterLogin() {
  const redirect =
    typeof router.currentRoute.value.query.redirect === 'string'
      ? router.currentRoute.value.query.redirect
      : '';

  if (redirect) {
    void router.push(redirect);
    return;
  }

  if (
    user.value?.role === 'client' ||
    user.value?.role === 'customer' ||
    user.value?.role === 'link'
  ) {
    void router.push({ name: 'FolderRootView' });
    return;
  }

  void router.push('/');
}

function usePasswordInstead() {
  showPassword.value = true;
  error.value = null;
}

function useDifferentEmail() {
  password.value = '';
  error.value = null;
  showPassword.value = false;
}

async function onEmailSubmit() {
  if (!passkeysSupported()) {
    usePasswordInstead();
    return;
  }

  loading.value = true;
  error.value = null;
  try {
    const response = await beginPasskeySignIn(email.value.trim());
    if (!response.passkey || !response.challenge_id || !response.public_key) {
      usePasswordInstead();
      return;
    }
    const credential = await getPasskeyAssertion(response.public_key);
    const ok = await auth.loginWithPasskey(email.value.trim(), response.challenge_id, credential);
    if (ok && user.value) {
      redirectAfterLogin();
      return;
    }
    showPassword.value = true;
    error.value = auth.error || 'Passkey sign in failed. Use your password instead.';
  } catch (err) {
    showPassword.value = true;
    if (err instanceof Error && err.name !== 'NotAllowedError') {
      error.value = passkeyErrorMessage(err, 'Passkey sign in failed. Use your password instead.');
    }
  } finally {
    loading.value = false;
  }
}

async function onPasswordSubmit() {
  loading.value = true;
  error.value = null;
  const ok = await auth.login(email.value.trim(), password.value);
  loading.value = false;
  if (ok && user.value) {
    redirectAfterLogin();
  } else {
    error.value = auth.error;
  }
}

onMounted(async () => {
  const fragment = window.location.hash.startsWith('#')
    ? window.location.hash.slice(1)
    : window.location.hash;
  const params = new URLSearchParams(fragment);
  const token = params.get('token')?.trim() || '';
  if (!token) {
    return;
  }

  loading.value = true;
  error.value = null;
  const ok = await auth.acceptRedirectToken(token);
  loading.value = false;
  window.history.replaceState(
    {},
    document.title,
    window.location.pathname + window.location.search,
  );

  if (ok && user.value) {
    redirectAfterLogin();
    return;
  }

  error.value = auth.error || 'Sign in failed';
});
</script>

<template>
  <div
    class="flex sm:h-full pt-6 sm:pt-0 @max-xs:hidden sm:bg-gray-50 px-4 sm:items-center justify-center"
  >
    <div class="w-full max-w-md p-5 sm:p-6 bg-white rounded shadow">
      <div class="flex flex-col items-center justify-center">
        <img v-if="branding.logo" :src="branding.logo" alt="Logo" class="max-w-1/2" />
        <Logo v-else class="!text-7xl" />
        <h1 class="text-2xl font-bold mt-2 mb-6 text-center">Sign In</h1>
      </div>
      <form v-if="!showPassword" @submit.prevent="onEmailSubmit">
        <div class="mb-4">
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
          <span v-if="loading">Checking sign-in options...</span>
          <span v-else>Continue</span>
        </Button>
        <div v-if="error" class="mt-4 text-red-600 text-center">{{ error }}</div>
      </form>

      <form v-else @submit.prevent="onPasswordSubmit">
        <div class="mb-4">
          <FloatLabel>
            <label class="block mb-1 font-medium" for="email">Email</label>
            <InputText v-model="email" id="email" type="email" required class="w-full" />
          </FloatLabel>
        </div>
        <div class="mb-6">
          <FloatLabel variant="on">
            <label class="block mb-1 font-medium" for="password">Password</label>
            <InputText v-model="password" type="password" id="password" required fluid />
          </FloatLabel>
        </div>
        <Button type="submit" class="w-full" :disabled="loading">
          <span v-if="loading">Signing in...</span>
          <span v-else>Sign In</span>
        </Button>
        <div class="mt-4 text-center">
          <router-link class="text-sm text-blue-600 hover:underline" to="/forgot-password"
            >Forgot password?</router-link
          >
        </div>
        <div class="mt-3 text-center">
          <button
            type="button"
            class="text-sm text-blue-600 hover:underline"
            @click="useDifferentEmail"
          >
            Use a different email
          </button>
        </div>
        <div v-if="error" class="mt-4 text-red-600 text-center">{{ error }}</div>
      </form>
    </div>
  </div>
</template>
