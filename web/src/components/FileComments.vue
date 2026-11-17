<script setup lang="ts">
import axios from 'axios';
import { Button, Select, Textarea } from 'openvue';
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue';

import FileCommentCard from '@/components/FileCommentCard.vue';
import { useFileComments } from '@/composables/useFileComments';
import { useAuthStore } from '@/stores/auth';
import type { FileComment, FileCommentRevision } from '@/types/models';

const props = defineProps<{ fileId: string }>();
const emit = defineEmits<{ close: [] }>();
const auth = useAuthStore();
const sessionKey = computed(() => `${auth.user?.session_id}:${auth.user?.id}:${auth.user?.role}`);
const {
  comments,
  loading,
  saving,
  ready,
  canInternal,
  nextCursor,
  error,
  load,
  post,
  edit,
  remove,
  reportError,
} = useFileComments(() => ({ fileId: props.fileId, sessionKey: sessionKey.value }));
const draft = ref('');
const visibility = ref('shared');
const draftKey = ref(crypto.randomUUID());
const editing = ref<FileComment | null>(null);
const editBody = ref('');
const removing = ref('');
const historyFor = ref('');
const history = ref<FileCommentRevision[]>([]);
const historyLoading = ref(false);
const historyError = ref('');
const moreHistory = ref(false);
const list = ref<HTMLElement | null>(null);
const composerId = computed(() => `file-comment-${props.fileId}`);
const length = computed(() => Array.from(draft.value).length);
const editLength = computed(() => Array.from(editBody.value).length);
const teamDraftUnavailable = computed(
  () => ready.value && !canInternal.value && visibility.value === 'team',
);
const audiences = [
  { label: 'Team only', value: 'team' },
  { label: 'Shared with file readers', value: 'shared' },
];
let historyGeneration = 0;
function closeHistory() {
  historyGeneration += 1;
  historyFor.value = '';
  history.value = [];
  historyLoading.value = false;
  historyError.value = '';
  moreHistory.value = false;
}
watch(
  [draft, visibility],
  () => {
    draftKey.value = crypto.randomUUID();
  },
  { flush: 'sync' },
);
watch(ready, async (value) => {
  if (value && !draft.value.trim()) {
    visibility.value = canInternal.value ? 'team' : 'shared';
  }
  if (value) {
    await nextTick();
    list.value?.scrollTo?.({ top: list.value.scrollHeight, behavior: 'instant' });
  }
});
watch(
  () => [props.fileId, sessionKey.value],
  () => {
    draft.value = '';
    editing.value = null;
    removing.value = '';
    closeHistory();
  },
);
watch(ready, (value) => {
  if (!value) {
    closeHistory();
  }
});

function date(value: string) {
  return new Date(value).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
}
async function showHistory(comment: FileComment, older = false) {
  if (!older && historyFor.value === comment.id) {
    closeHistory();
    return;
  }
  if (!older) {
    closeHistory();
    historyFor.value = comment.id;
  }
  historyGeneration += 1;
  const generation = historyGeneration;
  historyLoading.value = true;
  historyError.value = '';
  try {
    const { data } = await axios.get<FileCommentRevision[]>(
      `/api/files/${props.fileId}/comments/${comment.id}/history`,
      { params: older ? { before_version: history.value.at(-1)?.version } : {} },
    );
    if (generation !== historyGeneration) {
      return;
    }
    history.value = older ? [...history.value, ...data] : data;
    moreHistory.value = data.length === 50;
  } catch (cause) {
    if (generation !== historyGeneration) {
      return;
    }
    if (axios.isAxiosError(cause) && [401, 403, 404].includes(cause.response?.status || 0)) {
      closeHistory();
      reportError(cause);
    } else {
      historyError.value = 'History could not be loaded.';
    }
  } finally {
    if (generation === historyGeneration) {
      historyLoading.value = false;
    }
  }
}
async function send() {
  if (!draft.value.trim() || length.value > 4000 || teamDraftUnavailable.value) {
    return;
  }
  if (await post(draft.value.trim(), visibility.value, draftKey.value)) {
    draft.value = '';
    closeHistory();
    await nextTick();
    list.value?.scrollTo?.({ top: list.value.scrollHeight, behavior: 'instant' });
    document.getElementById(composerId.value)?.focus();
  }
}
async function startEdit(comment: FileComment) {
  editing.value = comment;
  editBody.value = comment.body;
  removing.value = '';
  await nextTick();
  document.getElementById(`${composerId.value}-edit`)?.focus();
}
async function saveEdit() {
  if (!editing.value || !editBody.value.trim() || editLength.value > 4000) {
    return;
  }
  if (await edit(editing.value, editBody.value.trim())) {
    editing.value = null;
    closeHistory();
  }
}
async function confirmRemove(comment: FileComment) {
  if (await remove(comment)) {
    removing.value = '';
    closeHistory();
  }
}
onBeforeUnmount(closeHistory);
</script>

<template>
  <section class="file-comments" aria-label="File comments">
    <header class="comments-heading">
      <div>
        <h2 :id="`${composerId}-heading`" tabindex="-1">Conversation</h2>
        <p>About this file</p>
      </div>
      <div class="comment-actions">
        <Button
          icon="oi oi-refresh"
          aria-label="Refresh comments"
          severity="secondary"
          variant="text"
          :disabled="loading || saving"
          @click="load()"
        />
        <Button
          icon="oi oi-times"
          aria-label="Close comments"
          severity="secondary"
          variant="text"
          @click="emit('close')"
        />
      </div>
    </header>
    <div ref="list" class="comments-list" :aria-busy="loading">
      <p v-if="loading && !ready" role="status" class="comment-state">Loading conversation…</p>
      <p v-else-if="ready && !comments.length" class="comment-state">
        No comments yet. Start a conversation about this file.
      </p>
      <Button
        v-if="nextCursor"
        label="Load earlier comments"
        severity="secondary"
        variant="text"
        :loading="loading"
        :disabled="saving"
        class="earlier-comments"
        @click="load(true)"
      />
      <ol aria-label="Comments" data-sentry-mask>
        <li v-for="comment in comments" :key="comment.id" class="comment">
          <FileCommentCard
            :comment="comment"
            :disabled="saving"
            :editing="editing?.id === comment.id"
            :historyOpen="historyFor === comment.id"
            @edit="startEdit(comment)"
            @remove="removing = comment.id"
            @history="showHistory(comment)"
          >
            <form v-if="editing?.id === comment.id" class="comment-edit" @submit.prevent="saveEdit">
              <label :for="`${composerId}-edit`"
                >Edit {{ comment.visibility === 'team' ? 'team note' : 'shared comment' }}</label
              >
              <Textarea
                :id="`${composerId}-edit`"
                v-model="editBody"
                rows="4"
                fluid
                :maxlength="8000"
                :disabled="saving"
                required
                :invalid="editLength > 4000"
              />
              <small>{{ editLength }}/4000 · Audience stays unchanged</small>
              <div class="comment-actions">
                <Button
                  type="submit"
                  label="Save edit"
                  size="small"
                  :loading="saving"
                  :disabled="!editBody.trim() || editLength > 4000"
                />
                <Button
                  label="Cancel edit"
                  severity="secondary"
                  variant="text"
                  size="small"
                  :disabled="saving"
                  @click="editing = null"
                />
              </div>
            </form>
            <p v-else class="comment-body" :class="{ removed: comment.deleted_at }">
              {{ comment.deleted_at ? 'Comment removed' : comment.body }}
            </p>
            <div
              v-if="removing === comment.id"
              class="comment-confirm"
              role="group"
              aria-label="Confirm comment removal"
            >
              <p>Remove this comment? It will remain in your private history.</p>
              <div class="comment-actions">
                <Button
                  label="Remove comment"
                  severity="danger"
                  size="small"
                  :loading="saving"
                  @click="confirmRemove(comment)"
                />
                <Button
                  label="Cancel"
                  severity="secondary"
                  variant="text"
                  size="small"
                  :disabled="saving"
                  @click="removing = ''"
                />
              </div>
            </div>
            <section
              v-if="historyFor === comment.id"
              class="comment-history"
              aria-label="Your comment history"
            >
              <p v-if="historyLoading" role="status">Loading history…</p>
              <p v-if="historyError" role="alert">{{ historyError }}</p>
              <ol>
                <li v-for="revision in history" :key="revision.id">
                  <small
                    >Version {{ revision.version }} · {{ revision.action }} ·
                    {{ date(revision.created_at) }}</small
                  >
                  <p class="comment-body">
                    {{ revision.action === 'removed' ? 'Comment removed' : revision.body }}
                  </p>
                </li>
              </ol>
              <Button
                v-if="moreHistory"
                label="Earlier versions"
                size="small"
                severity="secondary"
                variant="text"
                :disabled="historyLoading"
                @click="showHistory(comment, true)"
              />
            </section>
          </FileCommentCard>
        </li>
      </ol>
    </div>
    <p v-if="error" role="alert" class="comment-error">{{ error }}</p>
    <form v-if="ready" class="comment-composer" data-sentry-mask @submit.prevent="send">
      <label :for="composerId">{{
        visibility === 'team' ? 'Add a team note' : 'Add a shared comment'
      }}</label>
      <Select
        v-if="canInternal"
        v-model="visibility"
        :options="audiences"
        optionLabel="label"
        optionValue="value"
        ariaLabel="Comment audience"
        :disabled="saving"
        fluid
      />
      <p :id="`${composerId}-audience`" class="audience-help">
        {{
          visibility === 'team'
            ? 'Only staff with access to this file can see this note.'
            : 'Visible to signed-in people with access to this file.'
        }}
      </p>
      <p v-if="teamDraftUnavailable" role="status" class="audience-help">
        Team-note access changed. Copy your draft, or clear it before writing a shared comment.
        <Button
          label="Clear team draft"
          severity="secondary"
          variant="text"
          size="small"
          @click="
            draft = '';
            visibility = 'shared';
          "
        />
      </p>
      <Textarea
        :id="composerId"
        v-model="draft"
        rows="3"
        fluid
        :maxlength="8000"
        required
        :disabled="saving"
        :readonly="teamDraftUnavailable"
        :invalid="length > 4000"
        :aria-describedby="`${composerId}-audience ${composerId}-length`"
        placeholder="Write about this document…"
      />
      <div class="composer-actions">
        <small :id="`${composerId}-length`">{{ length }}/4000</small>
        <Button
          type="submit"
          :label="visibility === 'team' ? 'Post team note' : 'Post comment'"
          :loading="saving"
          :disabled="!draft.trim() || length > 4000 || loading || teamDraftUnavailable"
        />
      </div>
      <small class="notification-note"
        >Comments stay on this file. No email notifications are sent.</small
      >
    </form>
  </section>
</template>

<style scoped>
.file-comments {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  background: var(--p-content-background);
}
.comments-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 16px;
  border-bottom: 1px solid var(--p-content-border-color);
}
.comments-heading h2 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}
.comments-heading p,
.audience-help,
.notification-note,
.comment-state,
.comment-history small {
  color: var(--p-text-muted-color);
  font-size: 12px;
}
.comments-heading p {
  margin: 4px 0 0;
}
.comments-list {
  flex: 1;
  overflow: auto;
  min-height: 0;
  padding: 12px;
}
ol {
  padding: 0;
  margin: 0;
  list-style: none;
}
.comment {
  margin-bottom: 12px;
}
.comment-body {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-size: 14px;
  line-height: 1.6;
  margin: 0;
}
.removed {
  color: var(--p-text-muted-color);
  font-style: italic;
}
.comment-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
}
.comments-heading :deep(.p-button) {
  width: 40px;
  height: 40px;
}
.comment-composer {
  display: grid;
  gap: 8px;
  padding: 16px;
  border-top: 1px solid var(--p-content-border-color);
}
.comment-composer > label,
.comment-edit > label {
  font-size: 14px;
  font-weight: 500;
}
.audience-help {
  margin: 0;
  line-height: 1.5;
}
.composer-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.composer-actions small,
.comment-edit small {
  color: var(--p-text-muted-color);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}
.notification-note {
  line-height: 1.5;
}
.comment-edit {
  display: grid;
  gap: 8px;
}
.comment-confirm,
.comment-history {
  padding: 12px;
  margin-top: 8px;
  background: var(--p-surface-50);
  border-radius: 4px;
}
.comment-confirm p {
  margin: 0 0 8px;
  font-size: 13px;
}
.comment-history li + li {
  margin-top: 16px;
}
.comment-state {
  padding-block: 24px;
  line-height: 1.6;
}
.comment-error {
  padding: 12px 16px;
  margin: 0;
  font-size: 13px;
  color: var(--p-red-700);
  background: var(--p-red-50);
}
.earlier-comments {
  width: 100%;
  margin-top: 8px;
}
textarea {
  resize: vertical;
  max-height: 180px;
}
</style>
