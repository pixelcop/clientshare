<script setup lang="ts">
import { useEventListener, useMediaQuery } from '@vueuse/core';
import axios from 'axios';

import FileComments from '@/components/FileComments.vue';
import FilePreview from '@/components/FilePreview.vue';
import SharingDialog from '@/components/SharingDialog.vue';
import { useAuthStore } from '@/stores/auth';
import { useBrandingStore } from '@/stores/branding';
import { useFileManagerStore } from '@/stores/fileManager';
import type { FileItem } from '@/types/models';
import {
  canPreviewFile,
  formatDate,
  formatSize,
  getFileIcon,
  getFileType,
  getFileURL,
  getFileViewURL,
} from '@/types/util';

const { fileId } = defineProps<{
  fileId: string;
}>();

const auth = useAuthStore();
const router = useRouter();
const fileManager = useFileManagerStore();
const brandingStore = useBrandingStore();
const file = ref<FileItem | null>(null);
const loading = ref(false);
const showShare = ref(false);
const showComments = ref(false);
const desktop = useMediaQuery('(min-width: 1024px)');
const commentsVisited = ref(false);
const commentCount = ref<number | null>(null);
let countVersion = 0;
const hasComments = computed(() => (commentCount.value || 0) > 0);
const commentLabel = computed(() =>
  commentCount.value === null
    ? 'Comments'
    : `Comments, ${commentCount.value} ${commentCount.value === 1 ? 'comment' : 'comments'}`,
);
const error = ref('');
const downloadError = ref('');
const downloading = ref(false);
const copied = ref(false);
let copyTimer: ReturnType<typeof setTimeout> | undefined;
let requestVersion = 0;
let returning = false;
let returnWhenLoaded = false;

const downloadUrl = computed(() => (file.value ? getFileURL(file.value, true) : ''));
const previewUrl = computed(() => (file.value ? getFileViewURL(file.value) : ''));
const folderName = computed(() =>
  file.value?.folder_id === fileManager.context?.folder.id
    ? fileManager.context?.folder.filename
    : '',
);

const homeRoute = computed(() => {
  if (file.value?.folder_id) {
    return { name: 'FolderView', params: { folderId: file.value.folder_id } };
  }
  return { name: auth.user?.role === 'link' ? 'FolderRootView' : 'SharedWithMe' };
});

function goBack() {
  if (returning) {
    return;
  }
  const previous = router.options.history.state.back;
  // Restore real in-app navigation, but never send a fresh file link back to sign-in.
  if (typeof previous === 'string' && previous.startsWith('/') && !previous.startsWith('//')) {
    const destination = router.resolve(previous);
    if (destination.matched.length && destination.meta.public === false) {
      returning = true;
      router.back();
      return;
    }
  }
  if (loading.value) {
    returnWhenLoaded = true;
    return;
  }
  returning = true;
  void router.replace(homeRoute.value);
}

function closeComments() {
  showComments.value = false;
  document.getElementById('file-comments-toggle')?.focus();
}

function setCommentCount(count: number | null) {
  countVersion += 1;
  commentCount.value =
    typeof count === 'number' && Number.isSafeInteger(count) && count >= 0 ? count : null;
}
async function refreshCommentCount() {
  if (!file.value?.capabilities?.can_read || !auth.user || auth.user.role === 'link') {
    return;
  }
  const id = file.value.id;
  countVersion += 1;
  const version = countVersion;
  try {
    const { data } = await axios.get<{ count: number }>(`/api/files/${id}/comments/count`);
    if (version === countVersion && file.value?.id === id) {
      commentCount.value = Number.isSafeInteger(data.count) && data.count >= 0 ? data.count : null;
    }
  } catch (cause) {
    if (version !== countVersion) {
      return;
    }
    if (axios.isAxiosError(cause) && [401, 403, 404].includes(cause.response?.status || 0)) {
      setCommentCount(null);
      showComments.value = false;
      commentsVisited.value = false;
    }
  }
}

useEventListener(window, 'keydown', (event: KeyboardEvent) => {
  if (event.key !== 'Escape' || event.defaultPrevented || event.repeat || event.isComposing) {
    return;
  }
  // Dialogs (including sharing's nested confirmations) own Escape until dismissed.
  if (showShare.value || document.querySelector('[role="dialog"], [role="alertdialog"]')) {
    return;
  }
  if (
    (event.target as HTMLElement | null)?.closest?.(
      '[role="menu"], [role="menubar"], [role="listbox"]',
    )
  ) {
    return;
  }
  if (
    (event.target as HTMLElement | null)?.closest?.('input, textarea, [contenteditable="true"]')
  ) {
    return;
  }
  if (showComments.value) {
    event.preventDefault();
    closeComments();
    return;
  }
  event.preventDefault();
  goBack();
});

async function loadFile() {
  const version = ++requestVersion;
  returning = false;
  returnWhenLoaded = false;
  file.value = null;
  showShare.value = false;
  showComments.value = false;
  commentsVisited.value = false;
  setCommentCount(null);
  error.value = '';
  downloadError.value = '';
  copied.value = false;
  clearTimeout(copyTimer);
  loading.value = true;
  try {
    const current = await fileManager.fetchFile(fileId);
    if (version === requestVersion) {
      file.value = current;
      // Apply the responsive default once per file, not on resize: an explicit
      // choice to hide comments must not be undone while reading a document.
      showComments.value =
        desktop.value &&
        !!current.capabilities?.can_read &&
        !!auth.user &&
        auth.user.role !== 'link';
      commentsVisited.value = showComments.value;
      void refreshCommentCount();
    }
  } catch {
    if (version === requestVersion) {
      error.value = 'This file is unavailable. Access may have expired or been removed.';
    }
  } finally {
    if (version === requestVersion) {
      loading.value = false;
      if (returnWhenLoaded) {
        returnWhenLoaded = false;
        goBack();
      }
    }
  }
}

async function download() {
  if (!file.value || downloading.value) {
    return;
  }
  const id = file.value.id;
  downloading.value = true;
  downloadError.value = '';
  fileManager.error = '';
  try {
    await fileManager.downloadFile(file.value);
    if (file.value?.id === id && fileManager.error) {
      downloadError.value = 'The file couldn’t be downloaded. Please try again.';
    }
  } finally {
    fileManager.error = '';
    downloading.value = false;
  }
}

async function copyLink() {
  if (!file.value) {
    return;
  }
  const base = brandingStore.branding.effectivePublicBaseURL.trim() || window.location.origin;
  try {
    await navigator.clipboard.writeText(`${base.replace(/\/+$/, '')}/files/${file.value.id}`);
    copied.value = true;
    clearTimeout(copyTimer);
    copyTimer = setTimeout(() => {
      copied.value = false;
    }, 2000);
  } catch {
    downloadError.value =
      'The file address couldn’t be copied. Copy it from your browser’s address bar.';
  }
}

async function toggleComments() {
  commentsVisited.value = true;
  showComments.value = !showComments.value;
  if (showComments.value && file.value) {
    await nextTick();
    document.getElementById(`file-comment-${file.value.id}-region`)?.focus();
  }
}

watch(
  [() => fileId, () => auth.user?.session_id, () => auth.user?.id, () => auth.user?.role] as const,
  loadFile,
  { immediate: true },
);
onBeforeUnmount(() => {
  requestVersion += 1;
  countVersion += 1;
  clearTimeout(copyTimer);
});
</script>

<template>
  <main class="file-detail" aria-label="File viewer">
    <header class="file-header">
      <Button
        icon="oi oi-arrow-left"
        aria-label="Back to files"
        title="Back to files (Esc)"
        aria-keyshortcuts="Escape"
        severity="secondary"
        variant="text"
        class="file-back"
        @click="goBack"
      />
      <template v-if="file">
        <i :class="getFileIcon(file)" class="file-header-icon" aria-hidden="true" />
        <div data-sentry-mask class="file-heading">
          <h1 :title="file.filename">{{ file.filename }}</h1>
          <div class="file-metadata">
            <router-link v-if="file.folder_id && folderName" :to="homeRoute" class="file-folder">
              <i class="oi oi-folder" aria-hidden="true" />
              <span>{{ folderName }}</span>
            </router-link>
            <span>{{ formatSize(file.size) }}</span>
            <span :title="formatDate(file.uploaded_at)">
              Added
              {{
                new Date(file.uploaded_at).toLocaleDateString(undefined, { dateStyle: 'medium' })
              }}
            </span>
          </div>
        </div>
        <div class="file-header-actions">
          <Button
            v-if="file.capabilities?.can_read && auth.user?.role !== 'link'"
            id="file-comments-toggle"
            label="Comments"
            icon="oi oi-comment"
            class="file-comments-toggle"
            :severity="hasComments ? undefined : 'secondary'"
            :variant="showComments ? undefined : 'text'"
            :badge="commentCount === null ? undefined : String(commentCount)"
            :badgeSeverity="hasComments ? undefined : 'secondary'"
            :aria-label="commentLabel"
            :aria-pressed="showComments"
            :aria-expanded="showComments"
            aria-controls="file-comments-pane"
            @click="toggleComments"
          />
          <Button
            :icon="copied ? 'oi oi-check' : 'oi oi-link'"
            aria-label="Copy file address — access is still required"
            :title="copied ? 'Copied' : 'Copy file address — access is still required'"
            severity="secondary"
            variant="text"
            class="file-copy"
            @click="copyLink"
          />
          <span class="sr-only" role="status">{{ copied ? 'File address copied' : '' }}</span>
          <Button
            v-if="file.capabilities?.can_read"
            icon="oi oi-download"
            label="Download"
            class="file-download"
            aria-label="Download"
            title="Download"
            severity="secondary"
            variant="text"
            :loading="downloading"
            @click="download"
          />
          <Button
            v-if="file.capabilities?.can_share && auth.user?.role !== 'link'"
            label="Share"
            icon="oi oi-share-alt"
            class="file-share"
            aria-haspopup="dialog"
            :aria-expanded="showShare"
            :aria-controls="`sharing-item-${file.id}`"
            @click="showShare = true"
          />
        </div>
      </template>
      <h1 v-else>File viewer</h1>
    </header>
    <p v-if="downloadError" role="alert" class="file-download-error">{{ downloadError }}</p>
    <div class="file-workspace" :class="{ 'comments-open': showComments }">
      <div class="file-canvas">
        <p v-if="loading" role="status" class="file-state">Loading file…</p>
        <div v-else-if="error" class="file-state" role="alert">
          <i class="oi oi-file" aria-hidden="true" />
          <h2>File unavailable</h2>
          <p>{{ error }}</p>
          <Button label="Try again" severity="secondary" @click="loadFile" />
        </div>
        <template v-else-if="file">
          <FilePreview
            v-if="file.capabilities?.can_read && canPreviewFile(file)"
            :key="file.id"
            :file-type="getFileType(file)"
            :file-url="previewUrl"
            :download-url="downloadUrl"
            :file-name="file.filename"
            maximized
          />
          <div v-else class="file-state">
            <i :class="getFileIcon(file)" aria-hidden="true" />
            <h2>
              {{ file.capabilities?.can_read ? 'No preview available' : 'Preview unavailable' }}
            </h2>
            <p v-if="file.capabilities?.can_read">
              This file type can’t be previewed. Download the file to open it on your device.
            </p>
            <p v-else>You don’t have permission to view or download this file.</p>
            <Button
              v-if="file.capabilities?.can_read"
              label="Download"
              icon="oi oi-download"
              :loading="downloading"
              @click="download"
            />
          </div>
        </template>
      </div>
      <aside
        v-if="file && commentsVisited && file.capabilities?.can_read && auth.user?.role !== 'link'"
        v-show="showComments"
        id="file-comments-pane"
        class="file-comments-pane"
        aria-label="File conversation"
      >
        <FileComments
          :key="`${file.id}:${auth.user?.session_id}`"
          :fileId="file.id"
          :visible="showComments"
          @count="setCommentCount"
          @changed="refreshCommentCount"
        />
      </aside>
    </div>
    <SharingDialog
      v-if="showShare && file"
      :visible="showShare"
      :scope="{ kind: 'item', id: file.id, name: file.filename, itemType: file.type }"
      @close="showShare = false"
    />
  </main>
</template>

<style scoped>
.file-detail {
  display: flex;
  flex-direction: column;
  height: 100dvh;
  width: 100%;
  min-width: 0;
  color: var(--p-text-color);
  background: var(--p-content-background);
}
.file-header {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 12px;
  min-height: 64px;
  padding: 8px 16px 8px 8px;
  border-bottom: 1px solid var(--p-content-border-color);
}
.file-back {
  flex-shrink: 0;
  width: 44px;
  height: 44px;
}
.file-header-icon {
  font-size: 40px;
  color: var(--p-text-muted-color);
}
.file-heading {
  flex: 1;
  min-width: 0;
}
h1 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  line-height: 1.4;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.file-metadata {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 16px;
  margin-top: 4px;
  font-size: 13px;
  color: var(--p-text-muted-color);
}
.file-folder {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.file-folder span {
  overflow-wrap: anywhere;
}
.file-folder:hover {
  text-decoration: underline;
  text-underline-offset: 3px;
}
.file-header-actions {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: 12px;
}
.file-header-actions :deep(.p-button) {
  min-height: 44px;
}
.file-share {
  min-width: 100px;
}
.file-copy {
  width: 44px;
}
.file-download-error {
  margin: 0;
  padding: 12px 24px;
  color: var(--p-red-700);
  background: var(--p-red-50);
}
.file-workspace {
  display: flex;
  flex: 1;
  min-height: 0;
  min-width: 0;
}
.file-comments-pane {
  flex: 0 0 360px;
  width: 360px;
  min-width: 0;
  border-inline-start: 1px solid var(--p-content-border-color);
}
.file-canvas {
  display: flex;
  flex: 1;
  min-height: 0;
  min-width: 0;
  overflow: auto;
  background: var(--p-surface-100);
}
.file-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 16px;
  width: 100%;
  min-height: 320px;
  padding: 40px 24px;
  text-align: center;
}
.file-state > i {
  margin-bottom: 8px;
  font-size: 96px;
  color: var(--p-surface-400);
}
.file-state h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  text-wrap: balance;
}
.file-state p {
  margin: 0;
  max-width: 48ch;
  color: var(--p-text-muted-color);
  line-height: 1.6;
}
.file-state :deep(.p-button) {
  min-height: 44px;
}
.file-folder:focus-visible,
.file-comments-pane :deep(.file-comments:focus-visible) {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 3px;
}
@media (max-width: 640px) {
  .file-header {
    flex-wrap: wrap;
    gap: 8px;
    padding: 8px 12px;
  }
  .file-header-icon {
    display: none;
  }
  .file-heading {
    flex-basis: calc(100% - 56px);
  }
  .file-header-actions {
    justify-content: flex-end;
    width: 100%;
    gap: 4px;
  }
  .file-header-actions :deep(.p-button) {
    padding-inline: 8px;
  }
  .file-share {
    min-width: 0;
  }
  .file-download {
    width: 44px;
  }
  .file-download :deep(.p-button-label) {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
  }
  .file-metadata {
    font-size: 12px;
    gap: 4px 12px;
  }
}
@media (max-width: 1023px) {
  .comments-open .file-canvas {
    display: none;
  }
  .file-comments-pane {
    flex: 1;
    width: 100%;
    border-inline-start: 0;
  }
}
</style>
