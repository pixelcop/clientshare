<script setup lang="ts">
import { storeToRefs } from 'pinia';
import type { PageState } from 'primevue';

import ActivityFeedItem from '@/components/ActivityFeedItem.vue';
import { useActivitiesStore } from '@/stores/activities';
import { FeedStateRead, FeedStateUnread } from '@/types/models';

const activitiesStore = useActivitiesStore();
const { items, pagination, summary, currentState, loading, error } = storeToRefs(activitiesStore);

const pageSizeOptions = [10, 20, 50];
const first = computed(() => Math.max(0, (pagination.value.page - 1) * pagination.value.page_size));
const leavingItemCount = ref(0);
const isAnimatingRemoval = computed(() => leavingItemCount.value > 0);
const canClearNotifications = computed(() => summary.value.unread_count > 0 && !loading.value);

onMounted(() => {
  void activitiesStore.fetchFeedPage(1, pagination.value.page_size, currentState.value);
});

function handlePageChange(event: PageState) {
  void activitiesStore.fetchFeedPage(event.page + 1, event.rows, currentState.value);
}

function handleStateChange(state: 'unread' | 'read') {
  void activitiesStore.setFeedState(state);
}

function handleClearAllNotifications() {
  if (!canClearNotifications.value) {
    return;
  }
  void activitiesStore.clearAllNotifications();
}

function handleListBeforeLeave() {
  leavingItemCount.value += 1;
}

function handleListAfterLeave() {
  leavingItemCount.value = Math.max(0, leavingItemCount.value - 1);
}

const emptyStateMessage = computed(() =>
  currentState.value === FeedStateUnread ? 'No unread activity.' : 'No read activity yet.',
);
</script>

<template>
  <div class="activity-feed">
    <div class="mb-6 flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
      <div class="max-w-2xl">
        <h1 class="text-2xl font-bold tracking-tight">Activity</h1>
      </div>
    </div>

    <div
      v-if="error"
      class="mb-4 rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700"
    >
      {{ error }}
    </div>

    <Card class="overflow-hidden">
      <template #header>
        <div class="flex flex-col gap-4 border-b border-surface px-4 py-4 sm:px-5">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div class="inline-flex rounded-2xl bg-surface-100 dark:bg-surface-800 p-1">
              <button
                type="button"
                class="feed-tab"
                :class="{ 'feed-tab--active': currentState === FeedStateUnread }"
                @click="handleStateChange(FeedStateUnread)"
              >
                <span>Unread</span>
                <span class="feed-tab__count">{{ summary.unread_count }}</span>
              </button>
              <button
                type="button"
                class="feed-tab"
                :class="{ 'feed-tab--active': currentState === FeedStateRead }"
                @click="handleStateChange(FeedStateRead)"
              >
                <span>Read</span>
                <span class="feed-tab__count">{{ summary.read_count }}</span>
              </button>
            </div>
            <Button
              type="button"
              :disabled="!canClearNotifications"
              class="clear-all"
              @click="handleClearAllNotifications"
              outlined
            >
              Clear all
            </Button>
          </div>
        </div>
      </template>

      <template #content>
        <div
          v-if="items.length === 0 && !isAnimatingRemoval"
          class="px-4 py-12 text-center text-sm text-slate-500 sm:px-5"
        >
          {{ emptyStateMessage }}
        </div>

        <TransitionGroup
          tag="ul"
          name="feed-list"
          class="relative divide-y divide-surface"
          @before-leave="handleListBeforeLeave"
          @after-leave="handleListAfterLeave"
          @leave-cancelled="handleListAfterLeave"
        >
          <ActivityFeedItem v-for="item in items" :key="item.id" :item />
        </TransitionGroup>

        <div
          v-if="pagination.total_items > 0 && !loading"
          class="flex flex-col gap-3 border-t border-surface px-4 py-4 sm:flex-row sm:items-center sm:justify-between sm:px-5"
        >
          <div class="text-sm">
            Showing {{ (pagination.page - 1) * pagination.page_size + 1 }} to
            {{ Math.min(pagination.page * pagination.page_size, pagination.total_items) }} of
            {{ pagination.total_items }}
          </div>
          <Paginator
            :alwaysShow="false"
            :first="first"
            :rows="pagination.page_size"
            :totalRecords="pagination.total_items"
            :rowsPerPageOptions="pageSizeOptions"
            @page="handlePageChange"
          />
        </div>
      </template>
    </Card>
  </div>
</template>

<style scoped>
@reference '@/assets/tailwind.css';

.feed-tab {
  display: inline-flex;
  align-items: center;
  cursor: pointer;
  gap: 0.65rem;
  border-radius: 1rem;
  padding: 0.65rem 0.9rem;
  @apply dark:text-slate-500;
  font-size: 0.9rem;
  font-weight: 600;
  transition:
    background-color 150ms ease,
    color 150ms ease,
    box-shadow 150ms ease;
}

.feed-tab--active {
  @apply bg-surface-0 dark:bg-surface-400 dark:text-slate-950;
  box-shadow: 0 8px 18px -16px rgba(15, 23, 42, 0.55);
  .feed-tab__count {
    @apply dark:bg-surface-300/40;
  }
}

.feed-tab__count {
  min-width: 1.6rem;
  border-radius: 999px;
  background: rgba(148, 163, 184, 0.18);
  padding: 0.1rem 0.45rem;
  text-align: center;
  font-size: 0.72rem;
}

.feed-list-enter-active,
.feed-list-leave-active,
.feed-list-move {
  transition:
    opacity 180ms ease,
    transform 180ms ease;
}

.feed-list-enter-from,
.feed-list-leave-to {
  opacity: 0;
  transform: translateY(-10px);
}

.feed-list-leave-active {
  position: absolute;
  left: 0;
  right: 0;
}
</style>
