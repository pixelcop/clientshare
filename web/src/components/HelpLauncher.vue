<script setup lang="ts">
import axios from 'axios';
import { computed, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';

import HelpPanel from '@/components/HelpPanel.vue';
import { fetchHelpCapabilities, type HelpCapabilities } from '@/services/helpCapabilities';
import { useAuthStore } from '@/stores/auth';
import { useBrandingStore } from '@/stores/branding';
import { useCollectionStore } from '@/stores/collection';

const branding = useBrandingStore();
const collection = useCollectionStore();
const route = useRoute();
const auth = useAuthStore();
const guestRequest = computed(() => !auth.user && route.name === 'DocumentRequestRecipient');
const loginHelp = computed(
  () => !auth.user && ['Login', 'ForgotPassword', 'ResetPassword'].includes(String(route.name)),
);
const internal = computed(() => ['admin', 'manager'].includes(auth.user?.role || ''));
const email = ref('');
const floating = ref(false);
const visible = ref(false);
const dismissalKey = `clientshare-support-widget-dismissed:${auth.user?.id || 'guest'}`;
const dismissed = ref(false);
try {
  dismissed.value = localStorage.getItem(dismissalKey) === 'true';
} catch {
  // Restricted storage must not prevent access to support.
}

function dismissWidget() {
  if (!auth.user) {
    return;
  }
  dismissed.value = true;
  try {
    localStorage.setItem(dismissalKey, 'true');
  } catch {
    // The widget still stays hidden for the current page session.
  }
  document.getElementById('help-menu-trigger')?.focus();
}
const loading = ref(false);
const sending = ref(false);
const message = ref('');
const error = ref('');
const receipt = ref('');
const retry = ref<{ id: string; message: string; screen?: string; email?: string }>();
const capabilities = ref<HelpCapabilities>({ provider: 'none' });
const guideURL = computed(() => capabilities.value.guide_url || branding.branding.docsURL);

async function loadCapabilities() {
  loading.value = true;
  try {
    capabilities.value = await fetchHelpCapabilities(Boolean(auth.user));
  } catch {
    capabilities.value = { provider: 'none' };
  } finally {
    loading.value = false;
  }
}

onMounted(loadCapabilities);

function openSupport(fromWidget = false) {
  floating.value = fromWidget;
  if (!message.value.trim() && !sending.value) {
    error.value = '';
    receipt.value = '';
  }
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
  <div
    v-if="
      (internal || loginHelp) &&
      (!auth.user || !dismissed) &&
      (capabilities.provider === 'clientshare' || capabilities.support_url)
    "
    class="support-widget-launcher"
  >
    <div v-if="auth.user" class="support-widget-dismiss-slot">
      <Button
        icon="oi oi-times"
        iconClass="text-xs"
        aria-label="Dismiss help widget"
        title="Hide this widget. You can still contact support from Help."
        severity="secondary"
        text
        rounded
        class="support-widget-dismiss"
        @click="dismissWidget"
      />
    </div>
    <Button
      label="Need help?"
      icon="oi oi-comments"
      rounded
      :as="capabilities.provider === 'clientshare' ? 'button' : 'a'"
      :href="capabilities.support_url"
      :target="capabilities.support_url ? '_blank' : undefined"
      :rel="capabilities.support_url ? 'noopener noreferrer' : undefined"
      :aria-haspopup="capabilities.provider === 'clientshare' ? 'dialog' : undefined"
      :aria-expanded="visible && floating"
      @click="capabilities.provider === 'clientshare' && openSupport(true)"
    />
  </div>
  <HelpPanel
    v-model:visible="visible"
    :guideURL="guideURL"
    :supportLabel="capabilities.support_label"
    :supportURL="capabilities.support_url"
    :contactAvailable="!guestRequest && capabilities.provider === 'clientshare'"
    :loading="loading"
    :showLauncher="Boolean(auth.user) || guestRequest"
    :floating="floating"
    :gettingStarted="Boolean(auth.user) && collection.allowed"
    @start="collection.start()"
    @open="loadCapabilities"
    @contact="openSupport()"
  >
    <div v-if="receipt" class="flex flex-col gap-4" role="status">
      <p>Thanks for reaching out! We’ve received your message and will reply by email.</p>
      <Button
        label="Close"
        @click="
          visible = false;
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
    </form>
  </HelpPanel>
</template>

<style scoped>
.support-widget-launcher {
  position: fixed;
  right: max(1rem, env(safe-area-inset-right));
  bottom: max(1rem, env(safe-area-inset-bottom));
  z-index: 100;
  display: flex;
  align-items: center;
  padding: 0.25rem;
  border-radius: 9999px;
  background: var(--p-content-background);
  box-shadow: 0 4px 16px rgb(0 0 0 / 0.14);
}

.support-widget-dismiss-slot {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: flex-start;
  width: 0;
  overflow: hidden;
  opacity: 0;
  visibility: hidden;
  transition:
    width 180ms cubic-bezier(0.16, 1, 0.3, 1),
    opacity 120ms ease-out,
    visibility 0s 180ms;
}

.support-widget-launcher:is(:hover, :focus-within) .support-widget-dismiss-slot {
  width: 2.25rem;
  opacity: 1;
  visibility: visible;
  transition-delay: 0s;
}

.support-widget-dismiss {
  flex: 0 0 auto;
  width: 2rem;
  height: 2rem;
}

@media (hover: none) {
  .support-widget-launcher .support-widget-dismiss-slot {
    width: 44px;
    opacity: 1;
    visibility: visible;
  }

  .support-widget-dismiss {
    width: 44px;
    height: 44px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .support-widget-dismiss-slot {
    transition: none;
  }
}
</style>
