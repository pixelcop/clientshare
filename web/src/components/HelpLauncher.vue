<script setup lang="ts">
import axios from 'axios';
import { computed, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';

import HelpPanel from '@/components/HelpPanel.vue';
import { useAuthStore } from '@/stores/auth';
import { useBrandingStore } from '@/stores/branding';
import { useCollectionStore } from '@/stores/collection';

const branding = useBrandingStore();
const collection = useCollectionStore();
const route = useRoute();
const auth = useAuthStore();
const loginHelp = computed(
  () => !auth.user && ['Login', 'ForgotPassword', 'ResetPassword'].includes(String(route.name)),
);
const internal = computed(() => ['admin', 'manager'].includes(auth.user?.role || ''));
const email = ref('');
const floating = ref(false);
const visible = ref(false);
const contact = ref(false);
const loading = ref(false);
const sending = ref(false);
const message = ref('');
const error = ref('');
const receipt = ref('');
const retry = ref<{ id: string; message: string; screen?: string; email?: string }>();
const capabilities = ref<{
  provider: string;
  guide_url?: string;
  support_url?: string;
  support_label?: string;
  reply_email?: string;
  requester_name?: string;
  booking_url?: string;
}>({ provider: 'none' });
const guideURL = computed(() => capabilities.value.guide_url || branding.branding.docsURL);

async function loadCapabilities() {
  loading.value = true;
  try {
    capabilities.value = (await axios.get(auth.user ? '/api/help' : '/api/help/login')).data;
  } catch {
    capabilities.value = { provider: 'none' };
  } finally {
    if (capabilities.value.provider !== 'clientshare') {
      contact.value = false;
    }
    loading.value = false;
  }
}

onMounted(loadCapabilities);

function open() {
  floating.value = false;
  contact.value = Boolean(message.value.trim()) || sending.value;
  if (!contact.value) {
    error.value = '';
    receipt.value = '';
  }
  visible.value = true;
  void loadCapabilities();
}

function openWidget() {
  floating.value = true;
  contact.value = true;
  visible.value = true;
  void loadCapabilities();
}

async function send() {
  if (sending.value || !message.value.trim()) {
    return;
  }
  sending.value = true;
  error.value = '';
  if (
    !retry.value ||
    retry.value.message !== message.value.trim() ||
    (loginHelp.value && retry.value.email !== email.value.trim())
  ) {
    const section = route.path.split('/')[1] || 'portal';
    retry.value = {
      id: crypto.randomUUID(),
      message: message.value.trim(),
      ...(loginHelp.value
        ? { email: email.value.trim() }
        : {
            screen: ['clients', 'files', 'activity', 'users', 'settings'].includes(section)
              ? section
              : 'portal',
          }),
    };
  }
  try {
    const { data } = await axios.post(
      loginHelp.value ? '/api/help/login/messages' : '/api/help/messages',
      retry.value,
    );
    if (!data.received || !data.id) {
      throw new Error('No receipt');
    }
    receipt.value = data.id;
    message.value = '';
    retry.value = undefined;
  } catch {
    error.value =
      'We couldn’t confirm your message was received. Your draft is still here. Please try again.';
  } finally {
    sending.value = false;
  }
}
</script>

<template>
  <Button
    v-if="
      (internal || loginHelp) &&
      (capabilities.provider === 'clientshare' || capabilities.support_url)
    "
    class="support-widget-launcher"
    label="Need help?"
    icon="oi oi-comments"
    rounded
    raised
    aria-haspopup="dialog"
    :aria-expanded="visible && floating"
    @click="capabilities.provider === 'clientshare' ? openWidget() : open()"
  />
  <HelpPanel
    v-model:visible="visible"
    v-model:contact="contact"
    :guideURL="guideURL"
    :supportLabel="capabilities.support_label"
    :supportURL="capabilities.support_url"
    :contactAvailable="capabilities.provider === 'clientshare'"
    :loading="loading"
    :showLauncher="Boolean(auth.user)"
    :floating="floating"
    :gettingStarted="Boolean(auth.user) && collection.allowed"
    @start="collection.start()"
    @open="open"
  >
    <div v-if="receipt" class="flex flex-col gap-4" role="status">
      <p>Thanks for reaching out! We’ve received your message and will reply by email.</p>
      <Button
        label="Close"
        @click="
          visible = false;
          contact = false;
          receipt = '';
        "
      />
    </div>
    <form v-else class="flex flex-col gap-4" @submit.prevent="send">
      <p>
        Tell us what you need a hand with. Your message goes straight to the ClientShare support
        team.
      </p>
      <label for="support-message">How can we help?</label>
      <Textarea
        id="support-message"
        v-model="message"
        rows="6"
        maxlength="8000"
        required
        autofocus
        :disabled="sending"
        class="w-full"
      />
      <p v-if="error" role="alert">{{ error }}</p>
      <template v-if="loginHelp">
        <label for="support-email">Where should we reply?</label>
        <InputText
          id="support-email"
          v-model="email"
          type="email"
          autocomplete="email"
          maxlength="254"
          required
          :disabled="sending"
          class="w-full"
          placeholder="you@example.com"
        />
        <p class="text-sm">
          Don’t include passwords, sign-in codes, or confidential client information.
        </p>
      </template>
      <p class="text-sm">We’ll reply by email.</p>
      <Button
        type="submit"
        label="Send message"
        :loading="sending"
        :disabled="sending || !message.trim()"
      />
      <a
        v-if="internal && capabilities.booking_url"
        :href="capabilities.booking_url"
        target="_blank"
        rel="noopener noreferrer"
        class="underline"
        >Book a 20-minute setup session</a
      >
      <Button
        v-if="!floating"
        label="Back to Help"
        severity="secondary"
        text
        :disabled="sending"
        @click="contact = false"
      />
    </form>
  </HelpPanel>
</template>

<style scoped>
.support-widget-launcher {
  position: fixed;
  right: max(1rem, env(safe-area-inset-right));
  bottom: max(1rem, env(safe-area-inset-bottom));
  z-index: 100;
}
</style>
