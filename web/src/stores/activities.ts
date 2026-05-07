import axios from 'axios';
import { defineStore } from 'pinia';

import type { FeedEvent, FeedListResponse, FeedSummary, PaginationResponse } from '@/types/models';
import { defaultPagination } from '@/types/util';

type ActivityFeedState = 'unread' | 'read';

function getErrorMessage(error: unknown, fallback: string) {
  if (
    typeof error === 'object' &&
    error !== null &&
    'response' in error &&
    typeof error.response === 'object' &&
    error.response !== null &&
    'data' in error.response &&
    typeof error.response.data === 'object' &&
    error.response.data !== null &&
    'error' in error.response.data &&
    typeof error.response.data.error === 'string'
  ) {
    return error.response.data.error;
  }
  return fallback;
}

export const useActivitiesStore = defineStore('activities', () => {
  const items = ref<FeedEvent[]>([]);
  const pagination = ref<PaginationResponse>(defaultPagination());
  const summary = ref<FeedSummary>({ unread_count: 0, read_count: 0 });
  const currentState = ref<ActivityFeedState>('unread');
  const loading = ref(false);
  const error = ref<string | null>(null);

  async function fetchFeedPage(
    page = 1,
    pageSize = pagination.value.page_size,
    state = currentState.value,
  ) {
    loading.value = true;
    error.value = null;
    try {
      const res = await axios.get<FeedListResponse>('/api/feed', {
        params: {
          page,
          page_size: pageSize,
          state,
        },
      });
      items.value = res.data.items || [];
      pagination.value = res.data.pagination || defaultPagination();
      summary.value = res.data.summary || { unread_count: 0, read_count: 0 };
      currentState.value = state;
    } catch (e: unknown) {
      error.value = getErrorMessage(e, 'Failed to fetch activity feed');
    } finally {
      loading.value = false;
    }
  }

  async function setFeedState(state: ActivityFeedState) {
    if (currentState.value === state) {
      return;
    }
    await fetchFeedPage(1, pagination.value.page_size, state);
  }

  function removeReadItemFromUnreadPage(id: string) {
    if (currentState.value !== 'unread') {
      return;
    }

    const nextItems = items.value.filter((item) => item.id !== id);
    if (nextItems.length === items.value.length) {
      return;
    }

    items.value = nextItems;
    summary.value = {
      unread_count: Math.max(0, summary.value.unread_count - 1),
      read_count: summary.value.read_count + 1,
    };
    pagination.value = {
      ...pagination.value,
      total_items: Math.max(0, pagination.value.total_items - 1),
      items_per_page: Math.max(0, pagination.value.items_per_page - 1),
    };
  }

  async function markRead(id: string) {
    try {
      await axios.post(`/api/feed/${id}/read`);
      removeReadItemFromUnreadPage(id);
      await fetchFeedPage(pagination.value.page, pagination.value.page_size, currentState.value);
    } catch (e: unknown) {
      error.value = getErrorMessage(e, 'Failed to mark activity as read');
    }
  }

  async function clearAllNotifications() {
    try {
      await axios.post('/api/feed/read-all');
      const nextPage = currentState.value === 'unread' ? 1 : pagination.value.page;
      await fetchFeedPage(nextPage, pagination.value.page_size, currentState.value);
    } catch (e: unknown) {
      error.value = getErrorMessage(e, 'Failed to clear notifications');
    }
  }

  async function refreshFeedPage() {
    await fetchFeedPage(pagination.value.page, pagination.value.page_size, currentState.value);
  }

  return {
    items,
    pagination,
    summary,
    currentState,
    loading,
    error,
    fetchFeedPage,
    setFeedState,
    markRead,
    clearAllNotifications,
    refreshFeedPage,
  };
});
