<script setup lang="ts">
import axios from 'axios';
import { computed, onMounted, ref } from 'vue';

interface AccountRequest {
  id: string;
  title: string;
  client_name: string;
  needs_action: number;
  under_review: number;
}

const requests = ref<AccountRequest[]>([]);
const loading = ref(true);
const failed = ref(false);
const ordered = computed(() =>
  [...requests.value].sort((a, b) => Number(b.needs_action > 0) - Number(a.needs_action > 0)),
);

async function load() {
  loading.value = true;
  failed.value = false;
  try {
    const { data } = await axios.get<AccountRequest[]>('/api/my-document-requests');
    requests.value = data;
  } catch {
    failed.value = true;
  } finally {
    loading.value = false;
  }
}

onMounted(load);
</script>

<template>
  <main class="mx-auto grid max-w-3xl gap-4 leading-normal">
    <h1 class="m-0 text-2xl font-semibold tracking-tight">Document Requests</h1>
    <p v-if="loading" role="status">Loading document requests…</p>
    <div v-else-if="failed" role="alert" class="flex flex-wrap items-center gap-3">
      <p class="m-0">Could not load your document requests.</p>
      <Button type="button" label="Try again" severity="secondary" @click="load" />
    </div>
    <p v-else-if="!ordered.length" class="text-muted-color">
      Document requests addressed to you will appear here.
    </p>
    <ul
      v-else
      class="divide-y divide-surface-200 rounded-xl border border-surface-200 bg-surface-0 dark:divide-surface-700 dark:border-surface-700 dark:bg-surface-900"
    >
      <li v-for="request in ordered" :key="request.id">
        <RouterLink
          :to="{ name: 'DocumentRequestRecipient', query: { request: request.id } }"
          class="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 px-4 py-3 hover:bg-surface-50 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-primary dark:hover:bg-surface-800"
          data-sentry-mask
        >
          <span class="min-w-0 wrap-anywhere">
            <strong class="block">{{ request.title }}</strong>
            <span class="text-sm text-muted-color">{{ request.client_name }}</span>
          </span>
          <span
            class="text-sm"
            :class="request.needs_action ? 'font-semibold' : 'text-muted-color'"
          >
            {{
              request.needs_action === 1
                ? '1 item needs attention'
                : request.needs_action
                  ? `${request.needs_action} items need attention`
                  : request.under_review
                    ? 'Under review'
                    : 'All documents accepted'
            }}
          </span>
        </RouterLink>
      </li>
    </ul>
  </main>
</template>
