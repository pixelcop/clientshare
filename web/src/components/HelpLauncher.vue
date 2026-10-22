<script setup lang="ts">
import axios from 'axios';
import { computed, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';

import HelpPanel from '@/components/HelpPanel.vue';
import { useBrandingStore } from '@/stores/branding';

const branding = useBrandingStore();
const route = useRoute();
const visible = ref(false);
const contact = ref(false);
const loading = ref(false);
const sending = ref(false);
const message = ref('');
const error = ref('');
const receipt = ref('');
const retry = ref<{ id: string; message: string; screen: string }>();
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
    capabilities.value = (await axios.get('/api/help')).data;
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
  visible.value = true;
  void loadCapabilities();
}

async function send() {
  if (sending.value || !message.value.trim()) {
    return;
  }
  sending.value = true;
  error.value = '';
  if (!retry.value || retry.value.message !== message.value.trim()) {
    const section = route.path.split('/')[1] || 'portal';
    retry.value = {
      id: crypto.randomUUID(),
      message: message.value.trim(),
      screen: ['clients', 'files', 'activity', 'users', 'settings'].includes(section)
        ? section
        : 'portal',
    };
  }
  try {
    const { data } = await axios.post('/api/help/messages', retry.value);
    if (!data.received || !data.id) {
      throw new Error('No receipt');
    }
    receipt.value = data.id;
    message.value = '';
    retry.value = undefined;
  } catch {
    error.value = 'Your message could not be confirmed. Your draft is saved here. Please retry.';
  } finally {
    sending.value = false;
  }
}
</script>

<template>
  <HelpPanel
    v-model:visible="visible"
    v-model:contact="contact"
    :guideURL="guideURL"
    :supportLabel="capabilities.support_label"
    :supportURL="capabilities.support_url"
    :contactAvailable="capabilities.provider === 'clientshare'"
    :loading="loading"
    @open="open"
  >
    <div v-if="receipt" class="flex flex-col gap-4" role="status">
      <p>Your message has been received by ClientShare support.</p>
      <p class="break-all">Request ID: {{ receipt }}</p>
      <Button
        label="Back to Help"
        @click="
          contact = false;
          receipt = '';
        "
      />
    </div>
    <form v-else class="flex flex-col gap-4" @submit.prevent="send">
      <p>Get help with your portal or tell us what isn’t working.</p>
      <p class="font-semibold">Sending to ClientShare support</p>
      <p class="break-words">
        {{ capabilities.requester_name }} · Reply address: {{ capabilities.reply_email }}
      </p>
      <label for="support-message">What are you trying to do?</label>
      <Textarea
        id="support-message"
        v-model="message"
        rows="6"
        maxlength="8000"
        required
        :disabled="sending"
        class="w-full"
      />
      <p v-if="error" role="alert">{{ error }}</p>
      <Button
        type="submit"
        label="Send message"
        :loading="sending"
        :disabled="sending || !message.trim()"
      />
      <a
        v-if="capabilities.booking_url"
        :href="capabilities.booking_url"
        target="_blank"
        rel="noopener noreferrer"
        class="underline"
        >Book a 20-minute setup session</a
      >
      <Button
        label="Back to Help"
        severity="secondary"
        text
        :disabled="sending"
        @click="contact = false"
      />
    </form>
  </HelpPanel>
</template>
