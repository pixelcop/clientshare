<script setup lang="ts">
import axios from 'axios';
import Button from 'openvue/button';
import { computed, nextTick, ref } from 'vue';

import { type DocumentRequestItem, documentRequestLabels } from '@/types/documentRequests';

const props = defineProps<{
  item: DocumentRequestItem;
  clientId: string;
  requestId: string;
  fileId: string;
}>();
const emit = defineEmits<{ reviewed: [] }>();
const reason = ref('');
const editingReason = ref(false);
const reasonInput = ref<HTMLTextAreaElement>();
const busy = ref(false);
const error = ref('');
const notice = ref('');
const currentSubmission = computed(() =>
  props.item.files.some((file) => file.id === props.fileId && file.round === props.item.round),
);

async function askForReplacement() {
  editingReason.value = true;
  await nextTick();
  reasonInput.value?.focus();
}

async function review(status: 'accepted' | 'replacement') {
  if (busy.value || !currentSubmission.value || props.item.status !== 'submitted') {
    return;
  }
  busy.value = true;
  error.value = '';
  try {
    const path = `/api/clients/${encodeURIComponent(props.clientId)}/document-requests/${encodeURIComponent(props.requestId)}`;
    await axios.post(`${path}/items/${encodeURIComponent(props.item.id)}/review`, {
      round: props.item.round,
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
  <section aria-label="Current document review" class="review-bar" data-sentry-mask>
    <div class="review-bar-controls">
      <strong>{{ item.name }}</strong>
      <p v-if="notice" role="status">{{ notice }}</p>
      <p v-else-if="!currentSubmission">Earlier submission</p>
      <p v-else-if="item.status !== 'submitted'">{{ documentRequestLabels[item.status] }}</p>
      <div v-else-if="!editingReason" class="review-bar-actions">
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
    </div>
    <form v-if="editingReason" class="review-bar-form" @submit.prevent="review('replacement')">
      <label>
        Reason to show the recipient
        <textarea
          ref="reasonInput"
          v-model="reason"
          rows="2"
          maxlength="1000"
          required
          :disabled="busy"
        />
      </label>
      <div class="review-bar-actions">
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
    <p v-if="error" role="alert">{{ error }}</p>
  </section>
</template>

<style scoped>
.review-bar {
  padding: 6px 12px;
  font-size: 14px;
  line-height: 1.4;
  overflow-wrap: anywhere;
  border-bottom: 1px solid var(--p-content-border-color);
  background: var(--p-content-hover-background);
}
.review-bar-controls,
.review-bar-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 8px;
}
.review-bar-controls {
  justify-content: space-between;
}
.review-bar p {
  margin: 0;
  color: var(--p-text-muted-color);
}
.review-bar-form {
  max-width: 36rem;
  margin: 6px 0 2px;
}
.review-bar-form textarea {
  display: block;
  width: 100%;
  margin: 4px 0 6px;
  padding: 6px 8px;
  border: 1px solid var(--p-content-border-color);
  border-radius: 4px;
  background: var(--p-content-background);
}
.review-bar-form textarea:focus-visible {
  outline: 2px solid var(--p-primary-color);
}
@media (max-width: 1023px) {
  .review-bar-actions :deep(button) {
    min-height: 40px;
  }
}
</style>
