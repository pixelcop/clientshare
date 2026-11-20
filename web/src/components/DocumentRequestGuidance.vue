<script setup lang="ts">
import type { RecipientDocumentRequest } from '@/types/documentRequests';

defineProps<{ request: RecipientDocumentRequest }>();
</script>

<template>
  <section
    v-if="request.description || request.updates?.length"
    aria-label="Request guidance"
    class="space-y-3 rounded-lg border border-surface-200 bg-surface-0 px-4 py-3 leading-normal dark:border-surface-700 dark:bg-surface-900"
    data-sentry-mask
  >
    <p v-if="request.description" class="m-0 wrap-anywhere whitespace-pre-wrap">
      {{ request.description }}
    </p>
    <div
      v-for="update in request.updates"
      :key="update.id"
      class="border-t border-surface-200 pt-3 first:border-t-0 first:pt-0 dark:border-surface-700"
    >
      <h2 class="m-0 text-sm font-semibold">
        More documents requested
        <time :datetime="update.created_at" class="ml-1 font-normal text-muted-color">
          {{ new Date(update.created_at).toLocaleDateString() }}
        </time>
      </h2>
      <p class="mt-1 mb-0 wrap-anywhere whitespace-pre-wrap">{{ update.description }}</p>
    </div>
  </section>
</template>
