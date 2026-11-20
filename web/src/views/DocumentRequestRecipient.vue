<script setup lang="ts">
import axios from 'axios';
import Button from 'openvue/button';
import { computed, nextTick, onMounted, ref } from 'vue';
import { useRoute } from 'vue-router';

import {
  type DocumentRequestItem,
  documentRequestLabels,
  type RecipientDocumentRequest,
} from '@/types/documentRequests';

const route = useRoute();
const accountRequest = typeof route.query.request === 'string' ? route.query.request : '';
const endpoint = '/api/document-request';
const config = {
  headers: { 'X-Document-Request': new URLSearchParams(route.hash.slice(1)).get('token') ?? '' },
};
const request = ref<RecipientDocumentRequest>();
const loading = ref(true);
const unavailable = ref(false);
const error = ref('');
const itemError = ref<{ id: string; text: string }>();
const active = ref<{ id: string; label: string }>();
const announcement = ref('');
const heading = ref<HTMLElement>();

const canSubmit = (item: DocumentRequestItem) =>
  item.status === 'needed' || item.status === 'replacement';
const remaining = computed(() => request.value?.items.filter(canSubmit).length ?? 0);
const complete = computed(() => request.value?.items.every((item) => item.status === 'accepted'));
const ordered = computed(() =>
  [...(request.value?.items ?? [])].sort((a, b) => Number(canSubmit(b)) - Number(canSubmit(a))),
);

async function load() {
  if (!config.headers['X-Document-Request'] && !accountRequest) {
    unavailable.value = true;
    loading.value = false;
    return;
  }
  error.value = '';
  try {
    if (!config.headers['X-Document-Request']) {
      const { data } = await axios.get<{ token: string }>(
        `/api/my-document-requests/${encodeURIComponent(accountRequest)}/link`,
      );
      config.headers['X-Document-Request'] = data.token;
    }
    request.value = (await axios.get<RecipientDocumentRequest>(endpoint, config)).data;
  } catch (failure) {
    if (axios.isAxiosError(failure) && [401, 404].includes(failure.response?.status ?? 0)) {
      unavailable.value = true;
      request.value = undefined;
    } else {
      error.value = 'Could not refresh the checklist. Try again.';
    }
  } finally {
    loading.value = false;
  }
}

async function upload(item: DocumentRequestItem, event: Event) {
  const input = event.target as HTMLInputElement;
  const files = Array.from(input.files ?? []);
  input.value = '';
  if (active.value || !files.length) {
    return;
  }
  itemError.value = undefined;
  let failed = 0;
  try {
    for (const [index, file] of files.entries()) {
      active.value = { id: item.id, label: `Uploading file ${index + 1} of ${files.length}…` };
      const body = new FormData();
      body.append('file', file);
      body.append('round', String(item.round));
      try {
        await axios.post(`${endpoint}/items/${item.id}/files`, body, config);
      } catch (failure) {
        failed += 1;
        const status = axios.isAxiosError(failure) ? failure.response?.status : undefined;
        if (status === 404 || status === 409) {
          failed += files.length - index - 1;
          break;
        }
      }
    }
    if (failed > 0) {
      itemError.value = {
        id: item.id,
        text: `${failed === 1 ? 'A file could' : `${failed} files could`} not be uploaded. Check the files listed below and retry anything missing. Each item can hold up to 20 files.`,
      };
    }
    await load();
    if (!failed && !error.value) {
      announcement.value = `Files added to ${item.name}. Submit the item when it is ready.`;
    }
  } finally {
    active.value = undefined;
  }
}

async function submit(item: DocumentRequestItem) {
  active.value = { id: item.id, label: 'Submitting for review…' };
  itemError.value = undefined;
  try {
    await axios.post(`${endpoint}/items/${item.id}/submit`, { round: item.round }, config);
    announcement.value = `${item.name} submitted for review.`;
  } catch {
    itemError.value = { id: item.id, text: 'Could not submit this item. Refresh and try again.' };
  } finally {
    await load();
    active.value = undefined;
    await nextTick();
    heading.value?.focus();
  }
}

onMounted(load);
</script>

<template>
  <main class="mx-auto max-w-3xl pt-4 pb-10 leading-normal">
    <p v-if="loading" role="status">Loading document request…</p>
    <template v-else-if="unavailable">
      <h1 class="text-2xl font-semibold tracking-tight">This request is no longer available</h1>
      <p class="mt-2 text-muted-color">Ask the sender for an active document request link.</p>
    </template>
    <div v-if="error" role="alert" class="mb-4 flex flex-wrap items-center gap-3">
      <p>{{ error }}</p>
      <Button
        type="button"
        label="Try again"
        severity="secondary"
        :disabled="!!active"
        @click="load"
      />
    </div>
    <template v-if="request">
      <h1 class="m-0 text-3xl leading-tight font-semibold tracking-tight wrap-anywhere sm:text-4xl">
        {{ request.title }}
      </h1>
      <p class="mt-2 text-muted-color">Document request</p>
      <section
        aria-labelledby="document-checklist-status"
        class="mt-6 rounded-xl border border-surface-200 bg-surface-0 dark:border-surface-700 dark:bg-surface-900"
      >
        <div class="px-4 py-4 sm:px-6">
          <h2
            id="document-checklist-status"
            ref="heading"
            tabindex="-1"
            class="m-0 text-lg leading-tight font-semibold tracking-tight"
          >
            <template v-if="remaining === 1">1 item needs your attention</template>
            <template v-else-if="remaining">{{ remaining }} items need your attention</template>
            <template v-else-if="complete">All documents accepted</template>
            <template v-else>Nothing for you to do right now</template>
          </h2>
          <p v-if="remaining" class="mt-1 text-sm text-muted-color">
            Add the requested files, then submit each item when it is ready.
          </p>
          <p v-else-if="!complete" class="mt-1 text-sm text-muted-color">
            The sender is reviewing your documents.
          </p>
          <p role="status" class="sr-only">{{ announcement }}</p>
        </div>

        <ul
          class="divide-y divide-surface-200 border-t border-surface-200 dark:divide-surface-700 dark:border-surface-700"
          aria-label="Requested documents"
        >
          <li v-for="item in ordered" :key="item.id" class="px-4 py-4 sm:px-6">
            <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
              <h3 class="m-0 min-w-0 text-base leading-snug font-semibold wrap-anywhere">
                {{ item.name }}
              </h3>
              <span
                class="text-sm font-medium"
                :class="{
                  'text-amber-900 dark:text-amber-300': item.status === 'replacement',
                  'text-emerald-800 dark:text-emerald-300': item.status === 'accepted',
                  'text-sky-800 dark:text-sky-300': item.status === 'submitted',
                  'text-muted-color': item.status === 'needed',
                }"
              >
                {{ documentRequestLabels[item.status] }}
              </span>
            </div>
            <p
              v-if="item.instructions && canSubmit(item)"
              class="mt-2 text-sm text-muted-color wrap-anywhere whitespace-pre-wrap"
            >
              {{ item.instructions }}
            </p>
            <p
              v-if="item.status === 'replacement'"
              :id="`replacement-${item.id}`"
              class="mt-3 rounded-lg bg-amber-50 px-3 py-2.5 text-sm text-amber-950 wrap-anywhere whitespace-pre-wrap dark:bg-amber-950 dark:text-amber-100"
            >
              {{ item.reason }}
            </p>
            <p
              v-if="itemError?.id === item.id"
              role="alert"
              class="mt-3 text-sm text-red-800 dark:text-red-300"
            >
              {{ itemError.text }}
            </p>
            <ul
              v-if="item.files.length"
              class="mt-2 space-y-1 text-sm text-muted-color"
              :aria-label="`Uploaded files for ${item.name}`"
            >
              <li v-for="(file, index) in item.files" :key="index" class="wrap-anywhere">
                {{ file.name }}
              </li>
            </ul>
            <div v-if="canSubmit(item)" class="mt-3 flex flex-wrap items-center gap-3">
              <label
                class="inline-flex items-center"
                :class="active ? 'cursor-wait opacity-60' : 'cursor-pointer'"
              >
                <input
                  type="file"
                  multiple
                  class="peer sr-only"
                  :disabled="!!active"
                  :aria-label="`Choose files for ${item.name}`"
                  :aria-describedby="
                    item.status === 'replacement' ? `replacement-${item.id}` : undefined
                  "
                  @change="upload(item, $event)"
                />
                <span
                  class="inline-flex min-h-[44px] items-center rounded-md border border-sky-700 px-3 text-sm font-semibold text-sky-800 hover:bg-sky-50 peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-sky-700 dark:border-sky-400 dark:text-sky-200 dark:hover:bg-sky-950"
                >
                  {{
                    item.files.length
                      ? 'Add more files'
                      : item.status === 'replacement'
                        ? 'Add replacement'
                        : 'Add files'
                  }}
                </span>
              </label>
              <Button
                v-if="item.files.length"
                type="button"
                label="Submit for review"
                unstyled
                class="inline-flex min-h-[44px] cursor-pointer items-center rounded-md bg-sky-800 px-3 text-sm font-semibold text-white hover:bg-sky-900 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-sky-700 disabled:cursor-wait disabled:opacity-60"
                :aria-label="`Submit ${item.name} for review`"
                :disabled="!!active || !!error"
                @click="submit(item)"
              />
              <p
                v-if="active?.id === item.id"
                role="status"
                class="w-full text-sm text-muted-color"
              >
                {{ active.label }}
              </p>
            </div>
          </li>
        </ul>
      </section>
    </template>
  </main>
</template>
