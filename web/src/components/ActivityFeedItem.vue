<script setup lang="ts">
import { storeToRefs } from 'pinia';

import { useActivitiesStore } from '@/stores/activities';
import {
  type FeedEvent,
  FeedEventSourceTypeFileUploadBatch,
  FeedStateUnread,
} from '@/types/models';
import { formatDate, formatDateNatural } from '@/types/util';

const InviteAccepted = 'invite_accepted';
const FileUploaded = 'file_uploaded';
const MaxVisibleGroupedFiles = 3;

const { item } = defineProps<{
  item: FeedEvent;
}>();

const activitiesStore = useActivitiesStore();

const { currentState } = storeToRefs(activitiesStore);

function handleMarkRead(item: FeedEvent) {
  if (item.is_read) {
    return;
  }
  void activitiesStore.markRead(item.id);
}

function iconClass(item: FeedEvent) {
  return item.event_type === 'invite_accepted' ? 'pi pi-user-plus' : 'pi pi-upload';
}

function actorLabel(item: FeedEvent) {
  return item.actor_name || item.actor_email || 'Someone';
}

function subjectLabel(item: FeedEvent) {
  return item.subject_name || item.subject_email || 'A user';
}

function fileName(path: string) {
  const parts = path.split('/').filter(Boolean);
  return parts[parts.length - 1] || path;
}

const folderParts = computed(() => {
  if (!item.file_path) {
    return [];
  }
  return item.file_path.split('/');
});

const folderPath = computed(() => {
  if (folderParts.value.length <= 2) {
    // only has root + filename
    return '';
  }
  // only show the folder path, remove root (client slug) and filename
  return '/' + folderParts.value.slice(1, folderParts.value.length - 1).join('/');
});

const isFileUploaded = computed(() => item.event_type === FileUploaded);
const isInviteAccepted = computed(() => item.event_type === InviteAccepted);
const groupedFiles = computed(() => item.files || []);
const groupedFileCount = computed(() => item.file_count || groupedFiles.value.length);
const isGroupedFileUpload = computed(
  () =>
    isFileUploaded.value &&
    item.source_type === FeedEventSourceTypeFileUploadBatch &&
    groupedFileCount.value > 1,
);
const visibleGroupedFiles = computed(() => groupedFiles.value.slice(0, MaxVisibleGroupedFiles));
const hiddenGroupedFileCount = computed(() =>
  Math.max(0, groupedFiles.value.length - visibleGroupedFiles.value.length),
);
const canMarkRead = computed(() => !item.is_read && currentState.value === FeedStateUnread);

const primaryLabel = computed(() => {
  if (item.event_type === 'invite_accepted') {
    return subjectLabel(item);
  }
  if (isGroupedFileUpload.value) {
    return `${groupedFileCount.value} files`;
  }
  return fileName(item.file_path);
});

const actionLabel = computed(() => {
  if (item.event_type === 'invite_accepted') {
    return 'accepted invite';
  }
  return `uploaded by ${actorLabel(item)}`;
});
</script>

<template>
  <li
    class="group flex flex-col justify-between pb-2 not-first:pt-2 sm:flex-row"
    :class="{
      '': item.is_read,
      '': !item.is_read,
    }"
  >
    <div class="flex flex-row">
      <i class="pt-1 text-muted-color" :class="iconClass(item)" />
      <div class="ml-2 flex flex-col gap-1">
        <div v-if="isFileUploaded" class="flex flex-wrap gap-2">
          <span v-if="item.client_name">{{ item.client_name }}</span>
          <span v-if="folderPath" class="text-sm text-muted-color">
            {{ folderPath }}
          </span>
        </div>
        <div v-else-if="isInviteAccepted" class="flex flex-wrap gap-2">
          <span v-if="item.subject_email" class="text-sm text-muted-color">
            {{ item.subject_email }}
          </span>
        </div>

        <div v-if="primaryLabel" class="text-sm font-semibold">
          <a
            v-if="isFileUploaded && !isGroupedFileUpload && item.file_id"
            :href="`/files/${item.file_id}`"
            class="hover:underline"
          >
            {{ primaryLabel }}
          </a>
          <template v-else>
            {{ primaryLabel }}
          </template>
        </div>

        <div
          v-if="isGroupedFileUpload && visibleGroupedFiles.length > 0"
          class="flex flex-wrap gap-x-3 gap-y-1 text-sm text-muted-color"
        >
          <template v-for="file in visibleGroupedFiles" :key="file.id || file.path">
            <a v-if="file.id" :href="`/files/${file.id}`" class="hover:underline">
              {{ fileName(file.path) }}
            </a>
            <span v-else>{{ fileName(file.path) }}</span>
          </template>
          <span v-if="hiddenGroupedFileCount > 0">+{{ hiddenGroupedFileCount }} more</span>
        </div>
      </div>
    </div>

    <!-- right col -->
    <div class="flex flex-row items-center gap-16">
      <div class="text-muted-color">{{ actionLabel }}</div>
      <div v-if="canMarkRead" class="grid items-center justify-items-end">
        <div
          class="col-start-1 row-start-1 text-muted-color transition-opacity group-hover:opacity-0"
          :title="formatDate(item.created_at)"
        >
          {{ formatDateNatural(item.created_at) }}
        </div>
        <Button
          size="small"
          class="col-start-1 row-start-1 opacity-0 pointer-events-none transition-opacity group-hover:opacity-100 group-hover:pointer-events-auto"
          outlined
          icon="pi pi-check"
          label="Mark read"
          @click="handleMarkRead(item)"
        />
      </div>
      <div v-else class="text-muted-color" :title="formatDate(item.created_at)">
        {{ formatDateNatural(item.created_at) }}
      </div>
    </div>
  </li>
</template>
