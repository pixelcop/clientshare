<script setup lang="ts">
import axios from 'axios';
import { Button, InputText } from 'openvue';

import { useAuthStore } from '@/stores/auth';

const auth = useAuthStore();
const route = useRoute();
const router = useRouter();
const token = computed(() => new URLSearchParams(route.hash.slice(1)).get('token') || '');
const email = ref('');
const password = ref('');
const busy = ref(false);
const error = ref('');
async function signIn() {
  if (await auth.login(email.value, password.value)) {
    password.value = '';
  }
}
async function accept() {
  busy.value = true;
  error.value = '';
  try {
    const result = await axios.post<{ redirect_to: string }>('/api/invitations/accept', {
      token: token.value,
    });
    await auth.fetchMe(true);
    await router.replace(result.data.redirect_to || '/shared-with-me');
  } catch {
    error.value =
      'This invitation is unavailable for this account. Check the invited email, or ask for a new invitation.';
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <section class="accept-access">
    <h1>Accept invitation</h1>
    <p>Use the account with the email address that received this invitation.</p>
    <p v-if="!token" role="alert">
      This invitation link is incomplete. Open the full link from your email.
    </p>
    <template v-else-if="auth.user && auth.user.role !== 'link'">
      <p>
        Signed in as <strong data-sentry-mask>{{ auth.user.email }}</strong>
      </p>
      <div class="flex flex-wrap gap-2">
        <Button label="Accept invitation" :loading="busy" @click="accept" /><Button
          label="Use another account"
          severity="secondary"
          :disabled="busy"
          @click="auth.logout()"
        />
      </div>
    </template>
    <template v-else-if="auth.user?.role === 'link'">
      <p>
        You’re using a sharing link. Sign out, then sign in with the email address that received
        this invitation.
      </p>
      <Button label="Sign out of recipient view" severity="secondary" @click="auth.logout()" />
    </template>
    <form v-else @submit.prevent="signIn">
      <label for="invited-email">Email</label
      ><InputText
        id="invited-email"
        v-model="email"
        type="email"
        autocomplete="username"
        required
        fluid
      />
      <label for="invited-password">Password</label
      ><InputText
        id="invited-password"
        v-model="password"
        type="password"
        autocomplete="current-password"
        required
        fluid
      />
      <Button type="submit" label="Sign in" :loading="auth.loading" />
      <p v-if="auth.error" role="alert">{{ auth.error }}</p>
      <router-link to="/forgot-password">Forgot password?</router-link>
    </form>
    <p v-if="error" role="alert">{{ error }}</p>
  </section>
</template>

<style scoped>
.accept-access {
  max-width: 32rem;
  margin: 32px auto;
  color: var(--p-text-color);
}
h1 {
  font-size: 28px;
  font-weight: 600;
}
p {
  margin: 16px 0;
  line-height: 1.5;
  color: var(--p-text-muted-color);
}
form {
  display: grid;
  gap: 12px;
  margin-top: 24px;
}
label {
  font-size: 14px;
  font-weight: 600;
}
</style>
