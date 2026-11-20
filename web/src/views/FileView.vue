<script setup lang="ts">
import { useEventListener, useMediaQuery } from '@vueuse/core';
import axios from 'axios';
import { Menu, Popover } from 'openvue';
import type { MenuItem } from 'openvue/menuitem';

import DocumentRequestReviewActions from '@/components/DocumentRequestReviewActions.vue';
import DocumentRequestReviewPanel from '@/components/DocumentRequestReviewPanel.vue';
import FileComments from '@/components/FileComments.vue';
import FilePreview from '@/components/FilePreview.vue';
import SharingDialog from '@/components/SharingDialog.vue';
import { useAuthStore } from '@/stores/auth';
import { useBrandingStore } from '@/stores/branding';
import { useFileManagerStore } from '@/stores/fileManager';
import type { DocumentRequest } from '@/types/documentRequests';
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
import { commentIdFromHash } from '@/utils/commentLink';

const { fileId } = defineProps<{
  fileId: string;
}>();

const auth = useAuthStore();
const router = useRouter();
const route = useRoute();
const commentTarget = computed(() => commentIdFromHash(router.currentRoute.value.hash));
const fileManager = useFileManagerStore();
const brandingStore = useBrandingStore();
const file = ref<FileItem | null>(null);
const recipientFile = computed(() => route.name === 'DocumentRequestFile');
const guestToken = ref('');
const recipientName = ref('');
const guestPreview = ref('');
const guestPreviewLoading = ref(false);
let previewController: AbortController | undefined;
const requestFileApi = computed(() => `/api/document-request/files/${encodeURIComponent(fileId)}`);
const guestHeaders = (token = guestToken.value) => ({ 'X-Document-Request': token });
const canComment = computed(
  () =>
    !!file.value?.capabilities?.can_read &&
    (!!guestToken.value || (!!auth.user && auth.user.role !== 'link')),
);
const loading = ref(false);
const showShare = ref(false);
const showComments = ref(false);
const desktop = useMediaQuery('(min-width: 1024px)');
const requestPopover = ref<InstanceType<typeof Popover>>();
const fileActionsMenu = ref<InstanceType<typeof Menu>>();
const requestDetails = ref<DocumentRequest>();
const requestContext = computed(() => {
  const { request, requestClient } = router.currentRoute.value.query;
  if (
    typeof request !== 'string' ||
    !request ||
    typeof requestClient !== 'string' ||
    !requestClient ||
    !['admin', 'manager'].includes(auth.user?.role ?? '')
  ) {
    return undefined;
  }
  return { requestId: request, clientId: requestClient };
});
const activeRequest = computed(() => {
  const request = requestDetails.value;
  if (request?.items.some((item) => item.files.some((entry) => entry.id === fileId))) {
    return request;
  }
  return undefined;
});
const mobileRequest = computed(() => !!activeRequest.value && !desktop.value);
const activeRequestItem = computed(() =>
  activeRequest.value?.items.find((item) => item.files.some((entry) => entry.id === fileId)),
);
const displayFileName = computed(
  () =>
    activeRequest.value?.items.flatMap((item) => item.files).find((entry) => entry.id === fileId)
      ?.name ??
    (recipientName.value || file.value?.filename || ''),
);
let checklistVersion = 0;

async function loadRequest() {
  const version = ++checklistVersion;
  const context = requestContext.value;
  if (!context) {
    return;
  }
  try {
    const { data } = await axios.get<DocumentRequest>(
      `/api/clients/${encodeURIComponent(context.clientId)}/document-requests/${encodeURIComponent(context.requestId)}`,
    );
    if (version === checklistVersion) {
      requestDetails.value = data;
    }
  } catch {
    if (version === checklistVersion) {
      requestDetails.value = undefined;
    }
  }
}
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
const canShare = computed(
  () => !!file.value?.capabilities?.can_share && auth.user?.role !== 'link',
);
let copyTimer: ReturnType<typeof setTimeout> | undefined;
let requestVersion = 0;
let returning = false;
let returnWhenLoaded = false;

const downloadUrl = computed(() => (file.value ? getFileURL(file.value, true) : ''));
const previewUrl = computed(() =>
  guestToken.value ? guestPreview.value : file.value ? getFileViewURL(file.value) : '',
);
const folderName = computed(() =>
  !recipientFile.value && file.value?.folder_id === fileManager.context?.folder.id
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
  if (recipientFile.value) {
    returning = true;
    const { request } = route.query;
    void router.replace({ name: 'DocumentRequestRecipient', query: { request }, hash: route.hash });
    return;
  }
  if (activeRequest.value && requestContext.value) {
    returning = true;
    void router.replace({ name: 'DocumentRequests', params: requestContext.value });
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
  if (!canComment.value || !file.value) {
    return;
  }
  const id = file.value.id;
  countVersion += 1;
  const version = countVersion;
  try {
    const { data } = await (guestToken.value
      ? axios.get<{ count: number }>(`${requestFileApi.value}/comments/count`, {
          headers: guestHeaders(),
        })
      : axios.get<{ count: number }>(`/api/files/${id}/comments/count`));
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

function clearGuestPreview() {
  previewController?.abort();
  if (guestPreview.value) {
    URL.revokeObjectURL(guestPreview.value);
  }
  guestPreview.value = '';
  guestPreviewLoading.value = false;
}

async function loadGuestPreview(token: string) {
  const controller = new AbortController();
  previewController = controller;
  guestPreviewLoading.value = true;
  try {
    const { data } = await axios.get<Blob>(`${requestFileApi.value}/view`, {
      headers: guestHeaders(token),
      responseType: 'blob',
      signal: controller.signal,
    });
    if (!controller.signal.aborted) {
      guestPreview.value = URL.createObjectURL(data);
    }
  } catch {
    // Offer the explicit download when the inline preview is unavailable.
  } finally {
    if (!controller.signal.aborted) {
      guestPreviewLoading.value = false;
    }
  }
}

async function loadFile() {
  const version = ++requestVersion;
  returning = false;
  returnWhenLoaded = false;
  file.value = null;
  recipientName.value = '';
  guestToken.value = '';
  clearGuestPreview();
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
    let current: FileItem;
    if (recipientFile.value) {
      let token = new URLSearchParams(route.hash.slice(1)).get('token');
      if (!token && typeof route.query.request === 'string') {
        const { data } = await axios.get<{ token: string }>(
          `/api/my-document-requests/${encodeURIComponent(route.query.request)}/link`,
        );
        token = data.token;
      }
      if (!token) {
        throw new Error('Missing document request');
      }
      const { data: scopedFile } = await axios.get<FileItem>(requestFileApi.value, {
        headers: guestHeaders(token),
      });
      if (version !== requestVersion) {
        return;
      }
      current = scopedFile;
      let useGuest = true;
      if (auth.user && auth.user.role !== 'link') {
        try {
          const { data } = await axios.get<FileItem>(`/api/files/${encodeURIComponent(fileId)}`);
          if (data.capabilities?.can_read) {
            current = data;
            useGuest = false;
          }
        } catch (cause) {
          if (
            !axios.isAxiosError(cause) ||
            ![401, 403, 404].includes(cause.response?.status ?? 0)
          ) {
            throw cause;
          }
        }
      }
      if (version !== requestVersion) {
        return;
      }
      recipientName.value = scopedFile.filename;
      if (useGuest) {
        guestToken.value = token;
        // Blob URLs do not carry the response's sandbox policy. Keep SVG
        // download-only for guests rather than exposing a navigable SVG blob.
        if (canPreviewFile(current) && getFileType(current) !== 'image/svg+xml') {
          void loadGuestPreview(token);
        }
      }
    } else {
      current = await fileManager.fetchFile(fileId);
    }
    if (version === requestVersion) {
      file.value = current;
      // Apply the responsive default once per file, not on resize: an explicit
      // choice to hide comments must not be undone while reading a document.
      showComments.value =
        (desktop.value || !!commentTarget.value || route.query.comments === '1') &&
        canComment.value;
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
    if (guestToken.value) {
      const { data } = await axios.get<Blob>(`${requestFileApi.value}/download`, {
        headers: guestHeaders(),
        responseType: 'blob',
      });
      if (file.value?.id === id) {
        const url = URL.createObjectURL(new Blob([data], { type: 'application/octet-stream' }));
        const anchor = document.createElement('a');
        anchor.href = url;
        anchor.download = displayFileName.value;
        anchor.click();
        setTimeout(() => URL.revokeObjectURL(url), 1000);
      }
      return;
    }
    await fileManager.downloadFile(file.value);
    if (file.value?.id === id && fileManager.error) {
      downloadError.value = 'The file couldn’t be downloaded. Please try again.';
    }
  } catch {
    if (file.value?.id === id) {
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

const requestFileActions = computed<MenuItem[]>(() => [
  ...(canShare.value
    ? [{ label: 'Share', icon: 'oi oi-share-alt', command: () => (showShare.value = true) }]
    : []),
  {
    label: 'Copy file address',
    icon: 'oi oi-link',
    command: () => void copyLink(),
  },
  ...(file.value?.capabilities?.can_read
    ? [
        {
          label: 'Download',
          icon: 'oi oi-download',
          disabled: downloading.value,
          command: () => void download(),
        },
      ]
    : []),
]);

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
watch(commentTarget, (target) => {
  if (target && canComment.value) {
    commentsVisited.value = true;
    showComments.value = true;
  }
});
watch(
  [requestContext, () => auth.user?.session_id],
  () => {
    requestDetails.value = undefined;
    void loadRequest();
  },
  { immediate: true },
);
watch([() => fileId, desktop], () => {
  requestPopover.value?.hide();
  fileActionsMenu.value?.hide();
});
onBeforeUnmount(() => {
  requestVersion += 1;
  countVersion += 1;
  checklistVersion += 1;
  clearTimeout(copyTimer);
  clearGuestPreview();
});
</script>

<template>
  <main class="file-detail" :class="{ 'mobile-request': mobileRequest }" aria-label="File viewer">
    <header class="file-header">
      <Button
        icon="oi oi-arrow-left"
        :aria-label="activeRequest || recipientFile ? 'Back to full request' : 'Back to files'"
        :title="
          activeRequest || recipientFile ? 'Back to full request (Esc)' : 'Back to files (Esc)'
        "
        aria-keyshortcuts="Escape"
        severity="secondary"
        variant="text"
        class="file-back"
        @click="goBack"
      />
      <template v-if="file">
        <i :class="getFileIcon(file)" class="file-header-icon" aria-hidden="true" />
        <div data-sentry-mask class="file-heading">
          <h1 :title="displayFileName">{{ displayFileName }}</h1>
          <div class="file-metadata">
            <router-link v-if="file.folder_id && folderName" :to="homeRoute" class="file-folder">
              <i class="oi oi-folder" aria-hidden="true" />
              <span>{{ folderName }}</span>
            </router-link>
            <span class="inline-flex items-baseline gap-2">
              <span v-if="file.folder_id && folderName" aria-hidden="true">·</span>
              <span>{{ formatSize(file.size) }}</span>
            </span>
            <span class="inline-flex items-baseline gap-2">
              <span aria-hidden="true">·</span>
              <span :title="formatDate(file.uploaded_at)">
                Added
                {{
                  new Date(file.uploaded_at).toLocaleDateString(undefined, { dateStyle: 'medium' })
                }}
              </span>
            </span>
          </div>
        </div>
        <div class="file-header-actions">
          <Button
            v-if="mobileRequest"
            label="Request"
            icon="oi oi-chevron-down"
            severity="secondary"
            variant="outlined"
            class="file-request-toggle"
            aria-haspopup="dialog"
            @click="requestPopover?.toggle($event)"
          />
          <Button
            v-if="canComment"
            id="file-comments-toggle"
            :label="mobileRequest ? undefined : 'Comments'"
            icon="oi oi-comment"
            class="file-comments-toggle"
            :severity="hasComments ? undefined : 'secondary'"
            :variant="showComments ? undefined : 'text'"
            :badge="commentCount === null ? undefined : String(commentCount)"
            :badgeSeverity="hasComments ? undefined : 'secondary'"
            :aria-label="commentLabel"
            :title="commentLabel"
            :aria-pressed="showComments"
            :aria-expanded="showComments"
            aria-controls="file-comments-pane"
            @click="toggleComments"
          />
          <Button
            v-if="!activeRequest && !guestToken"
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
            v-if="!activeRequest && file.capabilities?.can_read"
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
            v-if="!activeRequest && canShare"
            label="Share"
            icon="oi oi-share-alt"
            class="file-share"
            aria-label="Share"
            title="Share"
            aria-haspopup="dialog"
            :aria-expanded="showShare"
            :aria-controls="`sharing-item-${file.id}`"
            @click="showShare = true"
          />
          <template v-if="activeRequest">
            <Button
              :icon="copied ? 'oi oi-check' : 'oi oi-share-alt'"
              :label="mobileRequest ? undefined : 'File actions'"
              :severity="mobileRequest ? 'secondary' : undefined"
              class="file-request-actions"
              aria-label="File actions"
              title="Share, copy or download"
              aria-haspopup="menu"
              aria-controls="request-file-actions"
              @click="fileActionsMenu?.toggle($event)"
            />
            <Menu
              id="request-file-actions"
              ref="fileActionsMenu"
              :model="requestFileActions"
              aria-label="File actions"
              popup
            />
          </template>
        </div>
      </template>
      <h1 v-else>File viewer</h1>
    </header>
    <Popover
      v-if="mobileRequest && activeRequest && requestContext"
      ref="requestPopover"
      aria-label="Switch request documents"
      :style="{ width: 'min(340px, calc(100vw - 24px))' }"
      :pt="{
        content: { style: { padding: 0, maxHeight: 'min(60dvh, 480px)', overflowY: 'auto' } },
      }"
    >
      <DocumentRequestReviewPanel
        :key="fileId"
        :request="activeRequest"
        :client-id="requestContext.clientId"
        :file-id="fileId"
      />
    </Popover>
    <DocumentRequestReviewActions
      v-if="file && activeRequest && activeRequestItem && requestContext"
      :key="fileId"
      :item="activeRequestItem"
      :request-id="activeRequest.id"
      :client-id="requestContext.clientId"
      :file-id="fileId"
      @reviewed="loadRequest"
    />
    <p v-if="downloadError" role="alert" class="file-download-error">{{ downloadError }}</p>
    <div class="file-workspace" :class="{ 'comments-open': showComments }">
      <aside
        v-if="desktop && activeRequest && requestContext"
        class="file-request-pane"
        aria-label="Document request review"
      >
        <DocumentRequestReviewPanel
          :key="fileId"
          :request="activeRequest"
          :client-id="requestContext.clientId"
          :file-id="fileId"
        />
      </aside>
      <div class="file-canvas">
        <p v-if="loading" role="status" class="file-state">Loading file…</p>
        <div v-else-if="error" class="file-state" role="alert">
          <i class="oi oi-file" aria-hidden="true" />
          <h2>File unavailable</h2>
          <p>{{ error }}</p>
          <Button label="Try again" severity="secondary" @click="loadFile" />
        </div>
        <template v-else-if="file">
          <p v-if="guestPreviewLoading" role="status" class="file-state">Loading preview…</p>
          <FilePreview
            v-else-if="file.capabilities?.can_read && canPreviewFile(file) && previewUrl"
            :key="file.id"
            :file-type="getFileType(file)"
            :file-url="previewUrl"
            :download-url="guestToken ? undefined : downloadUrl"
            :file-name="displayFileName"
            @download="download"
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
        v-if="file && commentsVisited && canComment"
        v-show="showComments"
        id="file-comments-pane"
        class="file-comments-pane"
        aria-label="File conversation"
      >
        <FileComments
          :key="`${file.id}:${auth.user?.session_id}`"
          :fileId="file.id"
          :fileName="displayFileName"
          :visible="showComments"
          :focusId="commentTarget"
          :request-token="guestToken || undefined"
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
  gap: 4px 8px;
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
.file-request-pane {
  flex: 0 0 clamp(224px, 22vw, 280px);
  min-width: 0;
  overflow: auto;
  border-inline-end: 1px solid var(--p-content-border-color);
}
.mobile-request .file-header-actions {
  width: 100%;
  flex-wrap: wrap;
  gap: 4px;
}
.mobile-request .file-header {
  flex-wrap: wrap;
}
.mobile-request .file-heading {
  flex-basis: calc(100% - 112px);
}
.mobile-request .file-request-toggle {
  margin-inline-end: auto;
}
.mobile-request :is(.file-request-actions, .file-comments-toggle) {
  min-width: 44px;
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
    flex-wrap: wrap;
    width: 100%;
    gap: 4px;
  }
  .file-header-actions :deep(.p-button) {
    padding-inline: 8px;
  }
  .file-share {
    min-width: 0;
  }
  .file-metadata {
    font-size: 12px;
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
