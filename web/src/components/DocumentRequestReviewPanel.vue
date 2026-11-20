<script setup lang="ts">
import axios from 'axios';
import Button from 'openvue/button';
import { computed, nextTick, ref } from 'vue';

import {
  type DocumentRequest,
  documentRequestLabels,
  requestFileRoute,
} from '@/types/documentRequests';

const props = defineProps<{ request: DocumentRequest; clientId: string; fileId: string }>();
const emit = defineEmits<{ reviewed: [] }>();
const reason = ref('');
const editingReason = ref(false);
const reasonInput = ref<HTMLTextAreaElement>();
const busy = ref(false);
const error = ref('');
const notice = ref('');
const item = computed(() =>
  props.request.items.find((entry) => entry.files.some((file) => file.id === props.fileId)),
);
const currentSubmission = computed(() =>
  item.value?.files.some((file) => file.id === props.fileId && file.round === item.value?.round),
);

async function askForReplacement() {
  editingReason.value = true;
  await nextTick();
  reasonInput.value?.focus();
}

async function review(status: 'accepted' | 'replacement') {
  const current = item.value;
  if (busy.value || !currentSubmission.value || current?.status !== 'submitted') {
    return;
  }
  busy.value = true;
  error.value = '';
  try {
    const path = `/api/clients/${encodeURIComponent(props.clientId)}/document-requests/${encodeURIComponent(props.request.id)}`;
    await axios.post(`${path}/items/${encodeURIComponent(current.id)}/review`, {
      round: current.round,
      status,
      reason: status === 'replacement' ? reason.value : '',
    });
    notice.value = status === 'accepted' ? 'Document accepted.' : 'Replacement requested.';
    editingReason.value = false;
    reason.value = '';
    emit('reviewed');
  } catch {
    error.value = 'Could not save the review. Reopen the request to check the latest submission.';
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="request-review" data-sentry-mask>
    <div class="request-review-header">
      <h2>{{ request.title }}</h2>
      <RouterLink
        :to="{ name: 'DocumentRequests', params: { clientId, requestId: request.id } }"
        class="request-review-link"
      >
        Back to full request
      </RouterLink>
    </div>
    <div v-if="item" class="request-review-decision">
      <p>
        <strong>{{ item.name }}</strong>
      </p>
      <p v-if="!currentSubmission">Earlier submission</p>
      <template v-else-if="item.status === 'submitted' && !notice">
        <form
          v-if="editingReason"
          class="request-review-form"
          @submit.prevent="review('replacement')"
        >
          <label>
            Reason to show the recipient
            <textarea ref="reasonInput" v-model="reason" rows="3" maxlength="1000" required />
          </label>
          <div class="request-review-actions">
            <Button
              type="submit"
              label="Request replacement"
              size="small"
              :disabled="busy || !reason.trim()"
            />
            <Button
              type="button"
              label="Cancel"
              size="small"
              severity="secondary"
              variant="text"
              :disabled="busy"
              @click="editingReason = false"
            />
          </div>
        </form>
        <div v-else class="request-review-actions">
          <Button
            type="button"
            label="Accept"
            size="small"
            :disabled="busy"
            @click="review('accepted')"
          />
          <Button
            type="button"
            label="Ask for replacement"
            size="small"
            severity="secondary"
            variant="outlined"
            :disabled="busy"
            @click="askForReplacement"
          />
        </div>
      </template>
      <p v-else-if="!notice">{{ documentRequestLabels[item.status] }}</p>
      <p v-if="notice" role="status">{{ notice }}</p>
      <p v-if="error" role="alert">{{ error }}</p>
    </div>
    <nav aria-label="Request documents">
      <ul class="request-review-items">
        <li v-for="entry in request.items" :key="entry.id">
          <div class="request-review-item">
            <strong>{{ entry.name }}</strong>
            <span>{{ documentRequestLabels[entry.status] }}</span>
          </div>
          <ul
            v-if="
              entry.files.some(
                (file) => file.id && (file.round === entry.round || file.id === fileId),
              )
            "
          >
            <template v-for="file in entry.files" :key="file.id">
              <li v-if="file.id && (file.round === entry.round || file.id === fileId)">
                <RouterLink
                  :to="requestFileRoute(clientId, request.id, file.id)"
                  :aria-current="file.id === fileId ? 'page' : undefined"
                  class="request-review-file"
                  replace
                >
                  <i class="oi oi-file" aria-hidden="true" />{{ file.name }}
                </RouterLink>
              </li>
            </template>
          </ul>
        </li>
      </ul>
    </nav>
  </div>
</template>

<style scoped>
.request-review {
  overflow-wrap: anywhere;
  font-size: 14px;
  line-height: 1.45;
}
.request-review-header,
.request-review-decision,
.request-review-item {
  padding: 12px;
}
.request-review h2 {
  margin: 0 0 4px;
  font-size: 16px;
  line-height: 1.3;
}
.request-review p {
  margin: 0;
}
.request-review-decision {
  border-block: 1px solid var(--p-content-border-color);
  background: var(--p-content-hover-background);
}
.request-review-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}
.request-review-form textarea {
  display: block;
  width: 100%;
  margin-top: 4px;
  padding: 6px 8px;
  border: 1px solid var(--p-content-border-color);
  border-radius: 4px;
  background: var(--p-content-background);
}
.request-review-form label {
  display: block;
  margin-top: 8px;
}
.request-review-items > li + li {
  border-top: 1px solid var(--p-content-border-color);
}
.request-review-item span {
  display: block;
  color: var(--p-text-muted-color);
  font-size: 12px;
}
.request-review-link {
  text-decoration: underline;
  text-underline-offset: 2px;
}
.request-review-file {
  display: block;
  padding: 8px 12px;
}
.request-review-file i {
  margin-inline-end: 6px;
}
.request-review-file:hover,
.request-review-file[aria-current='page'] {
  background: var(--p-content-hover-background);
}
.request-review-file[aria-current='page'] {
  font-weight: 600;
}
.request-review :is(a, textarea):focus-visible {
  outline: 2px solid var(--p-primary-color);
  outline-offset: -2px;
}
</style>
