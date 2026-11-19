import axios from 'axios';
import { computed, onBeforeUnmount, ref, watch } from 'vue';

import type { FileComment } from '@/types/models';

interface CommentPage {
  items: FileComment[];
  next_cursor: string;
  can_internal: boolean;
  total_count: number;
  focus_unavailable?: boolean;
  focus_has_newer?: boolean;
}

// Conversation state belongs to the current file/session, never a shared cache.
export function useFileComments(
  context: () => { fileId: string; sessionKey: string; focusId?: string },
) {
  const comments = ref<FileComment[]>([]);
  const loading = ref(false);
  const saving = ref(false);
  const ready = ref(false);
  const totalCount = ref<number | null>(null);
  const canInternal = ref(false);
  const nextCursor = ref('');
  const error = ref('');
  const focusUnavailable = ref(false);
  const focusedWindow = ref(false);
  const focusHasNewer = ref(false);
  const unavailableMentionIds = ref<string[]>([]);
  const base = computed(() => `/api/files/${context().fileId}/comments`);
  let generation = 0;
  let listGeneration = 0;
  let controller: AbortController | undefined;

  function reset() {
    generation += 1;
    listGeneration += 1;
    controller?.abort();
    comments.value = [];
    loading.value = false;
    saving.value = false;
    ready.value = false;
    totalCount.value = null;
    canInternal.value = false;
    nextCursor.value = '';
    error.value = '';
    focusUnavailable.value = false;
    focusedWindow.value = false;
    focusHasNewer.value = false;
    unavailableMentionIds.value = [];
  }

  function failed(cause: unknown) {
    if (axios.isAxiosError(cause) && [401, 403, 404].includes(cause.response?.status || 0)) {
      reset();
      error.value = 'This conversation is unavailable. File access may have changed.';
    } else if (axios.isAxiosError(cause) && cause.response?.status === 409) {
      error.value =
        'This comment changed elsewhere. Cancel the edit, reopen the file, and try again.';
    } else if (axios.isAxiosError(cause) && cause.response?.status === 400) {
      const ids: unknown = cause.response.data?.unavailable_mention_ids;
      if (Array.isArray(ids) && ids.length && ids.every((id) => typeof id === 'string')) {
        unavailableMentionIds.value = ids;
        return;
      }
      error.value =
        'Check the comment length and mentions. Mentioned people must be available for this audience. Your draft has been kept.';
    } else {
      error.value = 'Comments could not be updated. Your draft has been kept. Please try again.';
    }
  }

  async function load(older = false, focusId = '') {
    if (loading.value || saving.value) {
      return;
    }
    const current = generation;
    listGeneration += 1;
    const request = listGeneration;
    controller = new AbortController();
    loading.value = true;
    error.value = '';
    try {
      const { data } = await axios.get<CommentPage>(base.value, {
        params: older ? { before: nextCursor.value } : { ...(focusId && { focus: focusId }) },
        signal: controller.signal,
      });
      if (current !== generation || request !== listGeneration) {
        return;
      }
      const rows = [...data.items].reverse();
      if (!older) {
        focusUnavailable.value = !!data.focus_unavailable;
        focusedWindow.value = !!focusId && !data.focus_unavailable;
        focusHasNewer.value = !!data.focus_has_newer;
      }
      const existing = data.can_internal
        ? comments.value
        : comments.value.filter((comment) => comment.visibility === 'shared');
      comments.value = older ? [...rows, ...existing] : rows;
      nextCursor.value = data.next_cursor;
      canInternal.value = data.can_internal;
      ready.value = true;
      totalCount.value = data.total_count;
    } catch (cause) {
      if (current === generation && request === listGeneration && !axios.isCancel(cause)) {
        failed(cause);
      }
    } finally {
      if (current === generation && request === listGeneration) {
        loading.value = false;
      }
    }
  }

  async function mutate(operation: () => Promise<{ data: FileComment }>): Promise<boolean> {
    if (!ready.value || saving.value) {
      return false;
    }
    const current = generation;
    saving.value = true;
    error.value = '';
    unavailableMentionIds.value = [];
    // A list response started before this mutation cannot overwrite its result.
    controller?.abort();
    listGeneration += 1;
    loading.value = false;
    try {
      const { data } = await operation();
      if (current !== generation) {
        return false;
      }
      comments.value = [...comments.value.filter((comment) => comment.id !== data.id), data].sort(
        (a, b) => a.id.localeCompare(b.id),
      );
      return true;
    } catch (cause) {
      if (current === generation) {
        failed(cause);
      }
      return false;
    } finally {
      if (current === generation) {
        saving.value = false;
      }
    }
  }

  function post(body: string, visibility: string, key: string) {
    const url = base.value;
    return mutate(() =>
      axios.post<FileComment>(
        url,
        { body, visibility },
        {
          headers: { 'Idempotency-Key': key },
        },
      ),
    );
  }
  function edit(comment: FileComment, body: string) {
    const url = `${base.value}/${comment.id}`;
    return mutate(() => axios.put<FileComment>(url, { body, version: comment.version }));
  }
  function remove(comment: FileComment) {
    const url = `${base.value}/${comment.id}`;
    return mutate(() => axios.delete<FileComment>(url, { data: { version: comment.version } }));
  }

  watch(
    () => [context().fileId, context().sessionKey, context().focusId],
    () => {
      reset();
      void load(false, context().focusId);
    },
    { immediate: true, flush: 'sync' },
  );
  onBeforeUnmount(reset);
  return {
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
    reportError: failed,
  };
}
