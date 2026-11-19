<script setup lang="ts">
import axios from 'axios';
import { Button, Checkbox, Panel } from 'openvue';
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, ref, watch } from 'vue';

import CommentBody from '@/components/CommentBody.vue';
import FileCommentCard from '@/components/FileCommentCard.vue';
import SharingDialog from '@/components/SharingDialog.vue';
import { useFileComments } from '@/composables/useFileComments';
import { useMentionSearch } from '@/composables/useMentionSearch';
import { useAuthStore } from '@/stores/auth';
import type { FileComment, FileCommentRevision } from '@/types/models';
import type { SharingRecipient } from '@/types/sharing';

const CommentEditor = defineAsyncComponent(() => import('@/components/CommentEditor.vue'));

const props = withDefaults(
  defineProps<{ fileId: string; fileName?: string; visible?: boolean; focusId?: string }>(),
  { visible: true, fileName: 'File' },
);
const emit = defineEmits<{ count: [value: number | null]; changed: [] }>();
const auth = useAuthStore();
const sessionKey = computed(
  () => `${auth.user?.tenant_id}:${auth.user?.session_id}:${auth.user?.id}:${auth.user?.role}`,
);
const mentions = useMentionSearch(() => ({ fileId: props.fileId, sessionKey: sessionKey.value }));
const {
  comments,
  loading,
  saving,
  ready,
  totalCount,
  canInternal,
  nextCursor,
  error,
  focusUnavailable,
  focusedWindow,
  focusHasNewer,
  unavailableMentionIds,
  load,
  post,
  edit,
  remove,
  reportError,
} = useFileComments(() => ({
  fileId: props.fileId,
  sessionKey: sessionKey.value,
  focusId: props.focusId,
}));
const draft = ref('');
const composerOpen = ref(false);
const visibility = ref('shared');
const draftKey = ref(crypto.randomUUID());
const editing = ref<FileComment | null>(null);
const editBody = ref('');
const shareTarget = ref<SharingRecipient | null>(null);
const shareStatus = ref('');
let finishSharing: ((granted: boolean) => void) | undefined;
const removing = ref('');
const historyFor = ref('');
const history = ref<FileCommentRevision[]>([]);
const historyLoading = ref(false);
const historyError = ref('');
const moreHistory = ref(false);
const list = ref<HTMLElement | null>(null);
const highlighted = ref('');
let highlightTimer: ReturnType<typeof setTimeout> | undefined;
const composerId = computed(() => `file-comment-${props.fileId}`);
const length = computed(() => Array.from(draft.value).length);
const editLength = computed(() => Array.from(editBody.value).length);
const teamDraftUnavailable = computed(
  () => ready.value && !canInternal.value && visibility.value === 'team',
);
const teamNoteTitle =
  'Only staff with access to this file can read team-only notes. Uncheck to share with signed-in file readers.';
let historyGeneration = 0;
function closeHistory() {
  historyGeneration += 1;
  historyFor.value = '';
  history.value = [];
  historyLoading.value = false;
  historyError.value = '';
  moreHistory.value = false;
}
function cancelSharing() {
  shareTarget.value = null;
  finishSharing?.(false);
  finishSharing = undefined;
}
function shareMention(person: SharingRecipient): Promise<boolean> {
  if (shareTarget.value) {
    return Promise.resolve(false);
  }
  shareStatus.value = '';
  shareTarget.value = person;
  return new Promise((resolve) => {
    finishSharing = resolve;
  });
}
function warmMentions() {
  if (props.visible && ready.value && (visibility.value === 'shared' || canInternal.value)) {
    mentions.warm(visibility.value);
  }
}
function refreshMentions() {
  mentions.reset();
  warmMentions();
}
async function invitationSaved(delivery: string) {
  const target = shareTarget.value;
  if (!target) {
    return;
  }
  const fileId = props.fileId;
  const session = sessionKey.value;
  const resolve = finishSharing;
  finishSharing = undefined;
  shareTarget.value = null;
  refreshMentions();
  let granted = false;
  // Existing file invitations are pending until accepted. A recheck is also
  // useful if access was granted concurrently in another session.
  try {
    await axios.post(`/api/files/${fileId}/comments/preview`, {
      body: `[@Person](mention:${target.id})`,
      visibility: 'shared',
    });
    if (fileId === props.fileId && session === sessionKey.value) {
      granted = true;
      shareStatus.value = `${target.name} now has access.`;
    }
  } catch {
    if (fileId === props.fileId && session === sessionKey.value) {
      shareStatus.value = `${delivery} You can mention ${target.name} once they have access.`;
    }
  } finally {
    resolve?.(granted);
  }
}
watch(totalCount, (count) => emit('count', count));
watch(error, (message) => {
  if (message && !ready.value) {
    emit('count', null);
  }
});
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
  if (value && !props.focusId) {
    await nextTick();
    list.value?.scrollTo?.({ top: list.value.scrollHeight, behavior: 'instant' });
  }
});
watch([ready, focusedWindow, () => props.focusId], async ([loaded, focused, target]) => {
  clearTimeout(highlightTimer);
  highlighted.value = '';
  if (!loaded || !focused || !target) {
    return;
  }
  await nextTick();
  if (!ready.value || !focusedWindow.value || target !== props.focusId) {
    return;
  }
  const row = document.getElementById(`comment-${target}`);
  if (row && list.value?.contains(row)) {
    row.scrollIntoView?.({ block: 'nearest', behavior: 'instant' });
    highlighted.value = target;
    highlightTimer = setTimeout(() => {
      highlighted.value = '';
    }, 2200);
  }
});
async function latestComments() {
  await load();
  if (!focusedWindow.value) {
    list.value?.scrollTo?.({ top: list.value.scrollHeight, behavior: 'instant' });
  }
}
watch(
  () => [props.fileId, sessionKey.value],
  () => {
    draft.value = '';
    composerOpen.value = false;
    editing.value = null;
    removing.value = '';
    shareStatus.value = '';
    cancelSharing();
    closeHistory();
  },
);
watch(ready, (value) => {
  if (!value) {
    closeHistory();
    mentions.reset();
  }
});
watch([ready, visibility, () => props.visible], warmMentions);
watch(unavailableMentionIds, (ids) => {
  if (ids.length) {
    refreshMentions();
  }
});
watch(visibility, () => {
  unavailableMentionIds.value = [];
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
  if (await post(draft.value, visibility.value, draftKey.value)) {
    refreshMentions();
    draft.value = '';
    composerOpen.value = false;
    emit('changed');
    closeHistory();
    await nextTick();
    list.value?.scrollTo?.({ top: list.value.scrollHeight, behavior: 'instant' });
    document.getElementById(`${composerId.value}-toggle`)?.focus();
  }
}
async function startEdit(comment: FileComment) {
  unavailableMentionIds.value = [];
  composerOpen.value = false;
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
  if (await edit(editing.value, editBody.value)) {
    refreshMentions();
    editing.value = null;
    emit('changed');
    closeHistory();
  }
}
async function confirmRemove(comment: FileComment) {
  if (await remove(comment)) {
    refreshMentions();
    removing.value = '';
    emit('changed');
    closeHistory();
  }
}
async function toggleComposer() {
  composerOpen.value = !composerOpen.value;
  await nextTick();
  document
    .getElementById(composerOpen.value ? composerId.value : `${composerId.value}-toggle`)
    ?.focus();
}
onBeforeUnmount(closeHistory);
onBeforeUnmount(cancelSharing);
onBeforeUnmount(() => clearTimeout(highlightTimer));
</script>

<template>
  <section
    :id="`${composerId}-region`"
    class="file-comments"
    aria-label="File comments"
    tabindex="-1"
  >
    <p v-if="shareStatus" role="status" class="comment-state" data-sentry-mask>{{ shareStatus }}</p>
    <p v-if="focusUnavailable" role="status" class="comment-state">
      This comment is no longer available.
    </p>
    <button
      v-if="focusHasNewer"
      type="button"
      class="latest-comments"
      :disabled="loading || saving"
      @click="latestComments"
    >
      View latest comments
    </button>
    <div ref="list" class="comments-list" :aria-busy="loading">
      <p v-if="loading && !ready" role="status" class="comment-state">Loading conversation…</p>
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
        <li
          v-for="comment in comments"
          :id="`comment-${comment.id}`"
          :key="comment.id"
          class="comment"
          :class="{ 'comment-highlighted': comment.id === highlighted }"
        >
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
              <CommentEditor
                :id="`${composerId}-edit`"
                v-model="editBody"
                :fileId="fileId"
                :sessionKey="sessionKey"
                :visibility="comment.visibility"
                :mentionSearch="mentions"
                :mentionHTML="comment.body_html"
                :shareMention="shareMention"
                :unavailableMentionIds="unavailableMentionIds"
                @mentions-converted="unavailableMentionIds = []"
                :label="`Edit ${comment.visibility === 'team' ? 'team note' : 'shared comment'}`"
                :rows="4"
                :disabled="saving"
              />
              <div class="composer-actions">
                <Button
                  label="Cancel edit"
                  severity="secondary"
                  variant="text"
                  size="small"
                  :disabled="saving"
                  @click="editing = null"
                />
                <Button
                  type="submit"
                  label="Save edit"
                  size="small"
                  :loading="saving"
                  :disabled="!editBody.trim() || editLength > 4000"
                />
              </div>
            </form>
            <p v-else-if="comment.deleted_at" class="comment-body removed">Comment removed</p>
            <CommentBody
              v-else
              class="comment-body"
              :body="comment.body"
              :html="comment.body_html"
            />
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
                  <p v-if="revision.action === 'removed'" class="comment-body removed">
                    Comment removed
                  </p>
                  <CommentBody
                    v-else
                    class="comment-body"
                    :body="revision.body"
                    :html="revision.body_html"
                  />
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
      <p v-if="error" role="alert" class="comment-error">{{ error }}</p>
      <div class="comment-entry">
        <Button
          v-if="ready && !composerOpen && !editing"
          :id="`${composerId}-toggle`"
          label="Add comment"
          icon="oi oi-plus"
          severity="secondary"
          variant="text"
          size="small"
          :disabled="saving"
          :aria-expanded="false"
          :aria-controls="`${composerId}-form`"
          @click="toggleComposer"
        />
        <Panel
          v-if="ready && composerOpen"
          class="comment-composer-panel"
          :pt="{
            header: { style: { display: 'none' } },
            content: { style: { padding: '12px' } },
            contentContainer: { 'aria-labelledby': undefined, 'aria-label': 'Comment editor' },
          }"
        >
          <form
            :id="`${composerId}-form`"
            class="comment-composer"
            data-sentry-mask
            @submit.prevent="send"
          >
            <p v-if="teamDraftUnavailable" role="status" class="audience-help">
              Team-note access changed. Copy your draft, or clear it before writing a shared
              comment.
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
            <CommentEditor
              :id="composerId"
              v-model="draft"
              :fileId="fileId"
              :sessionKey="sessionKey"
              :visibility="visibility"
              :mentionSearch="mentions"
              :shareMention="shareMention"
              :unavailableMentionIds="unavailableMentionIds"
              @mentions-converted="unavailableMentionIds = []"
              :label="visibility === 'team' ? 'Add a team note' : 'Add a shared comment'"
              :disabled="saving"
              :readonly="teamDraftUnavailable"
            >
              <template #metadata>
                <label
                  v-if="canInternal"
                  :for="`${composerId}-team`"
                  class="team-note-toggle"
                  :title="teamNoteTitle"
                >
                  <Checkbox
                    v-model="visibility"
                    binary
                    trueValue="team"
                    falseValue="shared"
                    :inputId="`${composerId}-team`"
                    :disabled="saving"
                    :pt="{ input: { title: teamNoteTitle } }"
                  />
                  <span>Team-only note</span>
                </label>
              </template>
            </CommentEditor>
            <div class="composer-actions">
              <Button
                type="button"
                label="Cancel"
                severity="secondary"
                variant="text"
                size="small"
                :disabled="saving"
                @click="toggleComposer"
              />
              <Button
                type="submit"
                :label="visibility === 'team' ? 'Post team note' : 'Post comment'"
                :loading="saving"
                :disabled="!draft.trim() || length > 4000 || loading || teamDraftUnavailable"
              />
            </div>
          </form>
        </Panel>
      </div>
    </div>
    <SharingDialog
      v-if="shareTarget"
      visible
      :scope="{ kind: 'item', id: fileId, name: fileName, itemType: 'file' }"
      :recipient="shareTarget"
      @close="cancelSharing"
      @invited="invitationSaved"
    />
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
.audience-help,
.comment-state,
.comment-history small {
  color: var(--p-text-muted-color);
  font-size: 12px;
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
.comment-highlighted :deep(.comment-card) {
  background: var(--p-highlight-background);
}
.latest-comments {
  align-self: flex-end;
  margin: 4px 12px 0;
  min-height: 28px;
  color: var(--p-text-muted-color);
  font-size: 12px;
  text-decoration: underline;
  cursor: pointer;
}
.comment-body {
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
.comment-entry {
  margin-top: 12px;
}
.comment-composer {
  display: grid;
  gap: 8px;
}
.team-note-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 24px;
  font-size: 13px;
  font-weight: 400;
  cursor: pointer;
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
