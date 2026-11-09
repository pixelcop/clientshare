<script setup lang="ts">
import axios from 'axios';
import { Button, InputText } from 'openvue';

import { useCollectionStore } from '@/stores/collection';
import { useSecureLinksStore } from '@/stores/secureLinks';
import {
  formatLinkExpiry,
  isUsableLink,
  linkAccessDescription,
  linkAccessLabel,
} from '@/types/linkAccess';
import type { SecureLink } from '@/types/models';

const { link, targetName } = defineProps<{ link: SecureLink; targetName: string }>();
const links = useSecureLinksStore();
const collection = useCollectionStore();
const attempt = links.emailAttempt(link.id);
const recipient = toRef(attempt, 'recipient');
const resultTitle = ref<HTMLElement | null>(null);
const copying = ref(false);
const copyMessage = ref('');
const sending = toRef(attempt, 'sending');
const deliveryStatus = toRef(attempt, 'status');
const deliveryError = toRef(attempt, 'error');
const now = ref(Date.now());
let clock: ReturnType<typeof setInterval> | undefined;
const usable = computed(() => isUsableLink(link, now.value) && !!link.link_url);
onMounted(() => {
  resultTitle.value?.focus();
  clock = setInterval(() => {
    now.value = Date.now();
  }, 1000);
});
onBeforeUnmount(() => {
  if (clock) {
    clearInterval(clock);
  }
});

async function copy() {
  if (!usable.value || copying.value) {
    return;
  }
  copying.value = true;
  try {
    await navigator.clipboard.writeText(link.link_url!);
    copyMessage.value = 'Link copied';
    if (['write', 'readwrite'].includes(link.access_type)) {
      collection.shared(link.client_id, link.id, 'copied');
    }
  } catch {
    copyMessage.value = 'Could not copy. Select and copy the address below.';
  } finally {
    copying.value = false;
  }
}

async function email() {
  if (!usable.value || sending.value) {
    return;
  }
  const address = recipient.value.trim();
  sending.value = true;
  deliveryError.value = '';
  try {
    const result = await links.emailLink(link.id, address, attempt.key);
    deliveryStatus.value = result.status;
  } catch (error: unknown) {
    deliveryStatus.value =
      axios.isAxiosError(error) && error.response?.data?.status === 'unavailable'
        ? 'unavailable'
        : 'failed';
    deliveryError.value =
      (axios.isAxiosError(error) && error.response?.data?.error) ||
      'We could not confirm the email was sent. Try again or copy the link.';
  } finally {
    sending.value = false;
  }
}
watch(recipient, () => {
  attempt.key = crypto.randomUUID();
  deliveryStatus.value = null;
  deliveryError.value = '';
});
</script>

<template>
  <div class="link-result">
    <h3 ref="resultTitle" tabindex="-1">
      {{ usable ? 'Your link is ready' : 'This link is no longer available' }}
    </h3>
    <dl class="link-summary">
      <div>
        <dt>Sharing</dt>
        <dd>Entire client workspace · {{ targetName }}</dd>
      </div>
      <div>
        <dt>Audience</dt>
        <dd>Anyone with this link</dd>
      </div>
      <div>
        <dt>Permissions</dt>
        <dd>{{ linkAccessLabel(link.access_type) }}</dd>
      </div>
      <div>
        <dt>Expires</dt>
        <dd>{{ formatLinkExpiry(link.expires_at) }}</dd>
      </div>
    </dl>
    <p>{{ linkAccessDescription(link.access_type) }}</p>
    <details class="link-note">
      <summary>Link history</summary>
      <p>Created {{ formatLinkExpiry(link.created_at) }}.</p>
      <p>
        Last accessed:
        {{ link.last_accessed ? formatLinkExpiry(link.last_accessed) : 'Not recorded' }}.
      </p>
      <p v-if="link.email">
        Contact email: {{ link.email }}. This does not restrict who can use the link.
      </p>
      <p>Link activity does not tell you who read the documents.</p>
    </details>
    <template v-if="usable">
      <label :for="`link-url-${link.id}`">Link address</label>
      <InputText
        :id="`link-url-${link.id}`"
        :modelValue="link.link_url"
        readonly
        fluid
        @focus="($event.target as HTMLInputElement).select()"
      />
      <div class="link-result-actions">
        <Button
          :id="`collect-result-copy-${link.id}`"
          :label="copyMessage === 'Link copied' ? 'Copied' : 'Copy link'"
          icon="oi oi-copy"
          @click="copy"
        />
        <Button asChild v-slot="slotProps" severity="secondary">
          <a
            :class="slotProps.class"
            :href="link.link_url"
            target="_blank"
            rel="noopener noreferrer"
            >Open recipient view</a
          >
        </Button>
      </div>
      <p v-if="copyMessage" role="status">{{ copyMessage }}</p>
      <p class="link-note">
        Recipient view asks before switching away from your signed-in account.
      </p>
      <form class="link-email" @submit.prevent="email">
        <label :for="`link-recipient-${link.id}`">Email link to</label>
        <InputText
          :id="`link-recipient-${link.id}`"
          v-model="recipient"
          type="email"
          autocomplete="email"
          required
          fluid
          :disabled="sending"
        />
        <Button
          type="submit"
          :label="
            deliveryStatus === 'failed' || deliveryStatus === 'unavailable'
              ? 'Retry email'
              : 'Email link'
          "
          severity="secondary"
          :loading="sending"
          :disabled="deliveryStatus === 'queued' || deliveryStatus === 'sent'"
        />
        <p v-if="!deliveryStatus" class="link-note">
          Copying the link does not send an email. Use Email link to send it.
        </p>
        <p v-if="deliveryStatus === 'queued'" role="status">Email is waiting to send.</p>
        <p v-if="deliveryStatus === 'sent'" role="status">
          Email sent. Delivery has not been confirmed.
        </p>
        <p v-if="deliveryError" role="alert" class="link-error">{{ deliveryError }}</p>
      </form>
    </template>
  </div>
</template>

<style scoped>
.link-result {
  display: grid;
  gap: 12px;
  font-size: 14px;
}
.link-result h3 {
  margin: 0;
  font-size: 22px;
  font-weight: 600;
}
.link-result p {
  margin: 0;
}
.link-summary {
  display: grid;
  gap: 12px;
  padding: 16px;
  border: 1px solid var(--p-content-border-color);
  border-radius: 8px;
}
.link-summary dt {
  color: var(--p-text-muted-color);
  font-size: 12px;
  margin-bottom: 4px;
}
.link-summary dd {
  overflow-wrap: anywhere;
}
.link-result-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.link-result :deep(.p-button) {
  min-height: 44px;
}
.link-note {
  font-size: 12px;
  color: var(--p-text-muted-color);
}
.link-result summary {
  min-height: 44px;
  padding-block: 12px;
  cursor: pointer;
}
.link-email {
  display: grid;
  gap: 12px;
  border-top: 1px solid var(--p-content-border-color);
  padding-top: 16px;
}
.link-email .p-button {
  justify-self: start;
}
.link-error {
  color: var(--p-red-600);
}
</style>
