<script setup lang="ts">
import { storeToRefs } from 'pinia';
import { Button, InputText, Password } from 'primevue';

import { useAuthStore } from '@/stores/auth';
import { usePublicLinkStore } from '@/stores/publicLink';

const publicLink = usePublicLinkStore();
const auth = useAuthStore();
const { user } = storeToRefs(auth);
const router = useRouter();

const showRegister = ref(false);

async function submitRegistration(e: Event) {
  const form = e.target as HTMLFormElement;
  const formData = new FormData(form);
  const name = String(formData.get('name') || '').trim();
  const email = String(formData.get('email') || '').trim();
  const password = String(formData.get('password') || '').trim();
  if (!name || !email || !password) {
    return;
  }
  const ok = await publicLink.registerUser(name, email, password);
  if (ok) {
    form.reset();
    showRegister.value = false;
    await auth.fetchMe(true);
    auth.offerPasskeyEnrollment();
    await router.push({ name: 'FolderRootView' });
  }
}
</script>

<template>
  <section
    v-if="user?.role === 'link'"
    class="bg-white rounded-xl border border-slate-200 shadow-sm p-6"
  >
    <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
      <div>
        <h3 class="text-lg font-semibold text-slate-900">Create an account (optional)</h3>
        <p class="text-sm text-slate-600">
          Register to access your files without needing this link again.
        </p>
      </div>
      <Button
        @click="showRegister = !showRegister"
        :class="showRegister && 'hidden sm:inline-flex'"
        :severity="showRegister ? 'secondary' : 'success'"
      >
        {{ showRegister ? 'Cancel' : 'Create account' }}
      </Button>
    </div>

    <!-- Form -->
    <div v-if="showRegister" class="mt-4">
      <form class="grid gap-3 md:grid-cols-2" @submit.prevent="submitRegistration">
        <div class="md:col-span-1">
          <label class="block text-xs font-semibold text-slate-600 mb-1">Name</label>
          <InputText name="name" type="text" class="w-full" required />
        </div>
        <div class="md:col-span-1">
          <label class="block text-xs font-semibold text-slate-600 mb-1">Email</label>
          <InputText name="email" type="email" class="w-full" required />
        </div>
        <div class="md:col-span-1">
          <label class="block text-xs font-semibold text-slate-600 mb-1">Password</label>
          <Password name="password" required :feedback="false" :minlength="8" fluid />
        </div>
        <div class="md:col-span-2 flex items-center gap-3">
          <Button type="submit">Create account</Button>
          <router-link class="text-sm text-blue-600 hover:underline" to="/login">
            Already have an account?
          </router-link>
        </div>
      </form>
      <p v-if="publicLink.registerError" class="mt-2 text-sm text-red-600">
        {{ publicLink.registerError }}
      </p>
      <p v-if="publicLink.registerSuccess" class="mt-2 text-sm text-emerald-600">
        Account created. You can now sign in from the login page.
      </p>
    </div>
  </section>
</template>
