<script setup lang="ts">
import axios from 'axios';
import Button from 'openvue/button';
import { computed, nextTick, ref, watch } from 'vue';
import { useRouter } from 'vue-router';

import RecipientPicker from '@/components/RecipientPicker.vue';
import {
  type DocumentRequest,
  type DocumentRequestItem,
  documentRequestLabels,
  type DocumentRequestSummary,
} from '@/types/documentRequests';

const { clientId, requestId } = defineProps<{ clientId: string; requestId?: string }>();
const router = useRouter();
const endpoint = computed(() => `/api/clients/${encodeURIComponent(clientId)}/document-requests`);
const detail = computed(() => `${endpoint.value}/${encodeURIComponent(requestId ?? '')}`);
const requests = ref<DocumentRequestSummary[]>([]);
const request = ref<DocumentRequest>();
const loading = ref(false);
const saving = ref(false);
const error = ref('');
const notice = ref('');
const creating = ref(false);
const title = ref('');
const emailAll = ref(true);
const recipientEmail = ref('');
const recipientEmails = ref<string[]>([]);
const folderOverride = ref<string | null>(null);
const folderDraft = ref('');
const editingFolder = ref(false);
const folderInput = ref<HTMLInputElement>();
const folderEditButton = ref<HTMLButtonElement>();
const folderName = computed(() => folderOverride.value ?? title.value.trim());

async function editFolder() {
  folderDraft.value = folderName.value;
  editingFolder.value = true;
  await nextTick();
  folderInput.value?.focus();
  folderInput.value?.select();
}

async function finishFolder(save: boolean) {
  if (save) {
    folderOverride.value = folderDraft.value.trim() || null;
  }
  editingFolder.value = false;
  await nextTick();
  folderEditButton.value?.focus();
}

function addRecipient() {
  const address = recipientEmail.value.trim();
  if (!address || recipientEmails.value.length >= 100) {
    return;
  }
  if (!recipientEmails.value.some((entry) => entry.toLowerCase() === address.toLowerCase())) {
    recipientEmails.value.push(address);
  }
  recipientEmail.value = '';
}
let nextRow = 0;
const newRow = () => ({ key: nextRow++, name: '', instructions: '' });
const draft = ref([newRow()]);
const returning = ref('');
const reason = ref('');
const revoking = ref(false);
const ready = computed(
  () => request.value?.items.filter((item) => item.status === 'submitted').length ?? 0,
);
const complete = computed(() => request.value?.items.every((item) => item.status === 'accepted'));
const inputClass =
  'w-full rounded-md border border-surface-300 bg-surface-0 px-3 py-2 leading-normal focus-visible:outline-2 focus-visible:outline-primary dark:border-surface-600 dark:bg-surface-900';

function requestRoute(id?: string) {
  return { name: 'DocumentRequests', params: { clientId, requestId: id ?? '' } };
}

let generation = 0;
async function load() {
  const current = ++generation;
  loading.value = true;
  error.value = '';
  try {
    if (requestId) {
      const result = await axios.get<DocumentRequest>(detail.value);
      if (current === generation) {
        request.value = result.data;
      }
    } else {
      const result = await axios.get<DocumentRequestSummary[]>(endpoint.value);
      if (current === generation) {
        requests.value = result.data;
      }
    }
  } catch {
    if (current === generation) {
      request.value = undefined;
      requests.value = [];
      error.value = 'Could not load document requests. Check your access and try again.';
    }
  } finally {
    if (current === generation) {
      loading.value = false;
    }
  }
}

watch(
  () => [clientId, requestId],
  () => {
    request.value = undefined;
    requests.value = [];
    returning.value = '';
    reason.value = '';
    notice.value = '';
    revoking.value = false;
    creating.value = false;
    editingFolder.value = false;
    void load();
  },
  { immediate: true },
);

async function create() {
  if (editingFolder.value || (!emailAll.value && recipientEmail.value.trim())) {
    return;
  }
  saving.value = true;
  error.value = '';
  try {
    const { data } = await axios.post<DocumentRequest>(endpoint.value, {
      title: title.value,
      folder_name: folderName.value,
      email_all: emailAll.value,
      recipient_emails: emailAll.value ? [] : recipientEmails.value,
      items: draft.value.map(({ name, instructions }) => ({ name, instructions })),
    });
    title.value = '';
    recipientEmail.value = '';
    recipientEmails.value = [];
    emailAll.value = true;
    folderOverride.value = null;
    draft.value = [newRow()];
    await router.push(requestRoute(data.id));
    if (data.email_status === 'queued') {
      const count = data.email_count ?? 0;
      notice.value = `Request created. Email queued for ${count === 1 ? '1 client user' : `${count} client users`}.`;
    } else if (data.email_status === 'unavailable') {
      notice.value = 'Request created. Email is unavailable; copy and share the request link.';
    } else if (data.email_status === 'no_recipients') {
      notice.value =
        'Request created. No client users could be emailed; copy and share the request link.';
    }
  } catch (failure) {
    error.value =
      (axios.isAxiosError<{ error?: string }>(failure) && failure.response?.data?.error) ||
      'Could not create the request. Check the title and item names, then try again.';
  } finally {
    saving.value = false;
  }
}

async function copyLink() {
  if (!request.value?.link) {
    return;
  }
  notice.value = '';
  try {
    await navigator.clipboard.writeText(new URL(request.value.link, window.location.origin).href);
    notice.value = 'Link copied. Send it directly to the intended recipient.';
  } catch {
    notice.value =
      'Could not copy the link. Check your browser’s clipboard permissions and try again.';
  }
}

async function review(item: DocumentRequestItem, status: 'accepted' | 'replacement') {
  saving.value = true;
  error.value = '';
  notice.value = '';
  try {
    await axios.post(`${detail.value}/items/${item.id}/review`, {
      round: item.round,
      status,
      reason: status === 'replacement' ? reason.value : '',
    });
    returning.value = '';
    reason.value = '';
    await load();
    notice.value =
      status === 'accepted'
        ? `${item.name} accepted.`
        : `Replacement requested for ${item.name}. Share the same request link with the recipient.`;
  } catch {
    error.value =
      'Could not save your decision. Refresh to check the latest submission and try again.';
  } finally {
    saving.value = false;
  }
}

async function revoke() {
  saving.value = true;
  error.value = '';
  notice.value = '';
  try {
    await axios.post(`${detail.value}/revoke`);
    revoking.value = false;
    await load();
    notice.value = 'Request link revoked. Submitted files remain available to staff.';
  } catch {
    error.value = 'Could not revoke the link. Try again.';
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <section class="mx-auto max-w-4xl space-y-5 leading-normal">
    <RouterLink
      :to="requestId ? requestRoute() : { name: 'Clients' }"
      class="inline-block text-sm text-primary hover:underline"
    >
      {{ requestId ? 'All document requests' : 'All clients' }}
    </RouterLink>
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h1 class="m-0 text-2xl font-semibold tracking-tight wrap-anywhere">
        {{ request?.title ?? 'Document requests' }}
      </h1>
      <Button
        v-if="!requestId && !creating && !loading && !error"
        type="button"
        label="New request"
        @click="creating = true"
      />
    </div>

    <p v-if="loading" role="status">Loading document requests…</p>
    <p v-if="error" role="alert" class="text-red-700 dark:text-red-300">{{ error }}</p>
    <p v-if="notice" role="status">{{ notice }}</p>

    <form
      v-if="!requestId && creating"
      class="space-y-4 rounded-xl border border-surface-200 p-4 dark:border-surface-700"
      @submit.prevent="create"
    >
      <h2 class="text-lg font-semibold">New document request</h2>
      <label class="block space-y-1">
        <span class="font-medium">Request title</span>
        <input
          v-model="title"
          :class="inputClass"
          maxlength="160"
          required
          placeholder="e.g. October close"
        />
      </label>
      <div class="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
        <label v-if="editingFolder" for="request-folder" class="text-muted-color"
          >Destination folder:</label
        >
        <span v-else class="text-muted-color">Destination folder:</span>
        <template v-if="editingFolder">
          <input
            id="request-folder"
            ref="folderInput"
            v-model="folderDraft"
            :class="inputClass"
            class="max-w-64"
            maxlength="160"
            placeholder="Use request title"
            @keydown.enter.prevent="finishFolder(true)"
            @keydown.esc.prevent="finishFolder(false)"
          />
          <Button type="button" label="Done" size="small" @click="finishFolder(true)" />
          <Button
            type="button"
            label="Cancel"
            size="small"
            variant="text"
            @click="finishFolder(false)"
          />
        </template>
        <template v-else>
          <span class="font-medium wrap-anywhere">{{ folderName || 'Same as request title' }}</span>
          <button
            ref="folderEditButton"
            type="button"
            class="min-h-9 cursor-pointer rounded-sm px-2 font-medium underline underline-offset-2 focus-visible:outline-2 focus-visible:outline-primary"
            aria-label="Edit destination folder"
            @click="editFolder"
          >
            Edit
          </button>
        </template>
      </div>
      <fieldset
        v-for="(item, index) in draft"
        :key="item.key"
        class="space-y-2 border-t border-surface-200 pt-3 dark:border-surface-700"
      >
        <legend class="pr-2 text-sm font-medium">Item {{ index + 1 }}</legend>
        <label class="block space-y-1">
          <span>Document name</span>
          <input v-model="item.name" :class="inputClass" maxlength="160" required />
        </label>
        <label class="block space-y-1">
          <span>Instructions <span class="text-muted-color">(optional)</span></span>
          <textarea v-model="item.instructions" :class="inputClass" maxlength="1000" rows="2" />
        </label>
        <Button
          v-if="draft.length > 1"
          type="button"
          label="Remove item"
          severity="secondary"
          variant="text"
          :aria-label="`Remove item ${index + 1}`"
          @click="draft.splice(index, 1)"
        />
      </fieldset>
      <Button
        type="button"
        label="Add another item"
        severity="secondary"
        :disabled="draft.length >= 30"
        @click="draft.push(newRow())"
      />
      <fieldset class="space-y-2 border-t border-surface-200 pt-3 dark:border-surface-700">
        <legend class="pr-2 text-sm font-medium">Delivery</legend>
        <label class="inline-flex min-h-9 cursor-pointer items-center gap-2 font-medium">
          <input
            v-model="emailAll"
            type="checkbox"
            class="size-4 accent-primary"
            :disabled="saving"
          />
          Email all client users
        </label>
        <p v-if="emailAll" class="text-sm text-muted-color">
          Everyone with client access to this workspace will get a separate email.
        </p>
        <div v-if="!emailAll" class="space-y-2">
          <RecipientPicker
            v-model="recipientEmail"
            input-id="request-recipient"
            :client-id="clientId"
            current-client-only
            label="Find a client user to email"
            :disabled="saving || recipientEmails.length >= 100"
          >
            <template #action>
              <Button
                type="button"
                label="Add recipient"
                severity="secondary"
                :disabled="saving || !recipientEmail.trim() || recipientEmails.length >= 100"
                @click="addRecipient"
              />
            </template>
          </RecipientPicker>
          <ul
            v-if="recipientEmails.length"
            aria-label="Selected recipients"
            class="flex flex-wrap gap-2"
          >
            <li
              v-for="address in recipientEmails"
              :key="address"
              class="flex items-center gap-1 rounded-md bg-surface-100 pl-2 text-sm dark:bg-surface-800"
            >
              <span class="min-w-0 wrap-anywhere">{{ address }}</span>
              <Button
                label="Remove"
                :aria-label="`Remove ${address}`"
                type="button"
                size="small"
                variant="text"
                severity="secondary"
                :disabled="saving"
                @click="recipientEmails = recipientEmails.filter((entry) => entry !== address)"
              />
            </li>
          </ul>
          <p class="text-sm text-muted-color">
            {{
              recipientEmail.trim()
                ? 'Use Add recipient to include this address.'
                : 'Leave the list empty to copy and share the link yourself.'
            }}
          </p>
        </div>
      </fieldset>
      <div class="flex flex-wrap gap-2">
        <Button
          type="submit"
          label="Create request"
          :disabled="saving || editingFolder || (!emailAll && !!recipientEmail.trim())"
        />
        <Button
          type="button"
          label="Cancel"
          severity="secondary"
          variant="text"
          :disabled="saving"
          @click="creating = false"
        />
      </div>
    </form>

    <template v-else-if="!requestId && !loading && !error">
      <p v-if="!requests.length" class="text-muted-color">
        No requests yet. Create a checklist to collect specific documents.
      </p>
      <ul
        v-else
        class="divide-y divide-surface-200 border-y border-surface-200 dark:divide-surface-700 dark:border-surface-700"
      >
        <li
          v-for="entry in requests"
          :key="entry.id"
          class="flex flex-wrap items-center justify-between gap-2 py-3"
        >
          <RouterLink
            :to="requestRoute(entry.id)"
            class="font-medium text-primary hover:underline"
            >{{ entry.title }}</RouterLink
          >
          <span class="text-sm text-muted-color">
            {{
              entry.revoked
                ? 'Link revoked'
                : new Date(entry.expires_at) <= new Date()
                  ? 'Link expired'
                  : `Link expires ${new Date(entry.expires_at).toLocaleDateString()}`
            }}
          </span>
        </li>
      </ul>
    </template>

    <template v-if="request">
      <p v-if="request.folder_id" class="text-sm text-muted-color">
        Destination folder:
        <RouterLink
          :to="{ name: 'FolderView', params: { folderId: request.folder_id } }"
          class="text-primary underline"
        >
          {{ request.folder_name }}
        </RouterLink>
      </p>
      <div class="flex flex-wrap items-center gap-2">
        <Button v-if="request.link" label="Copy request link" type="button" @click="copyLink" />
        <Button
          type="button"
          label="Refresh"
          severity="secondary"
          :disabled="loading || saving"
          @click="load"
        />
        <Button
          v-if="request.link && !revoking"
          type="button"
          label="Revoke link"
          severity="secondary"
          variant="text"
          @click="revoking = true"
        />
      </div>
      <p class="text-sm text-muted-color">
        <template v-if="request.link">
          Anyone with the link can view the checklist and submit files. Expires
          {{ new Date(request.expires_at).toLocaleDateString() }}.
        </template>
        <template v-else>{{
          request.revoked ? 'The request link has been revoked.' : 'The request link has expired.'
        }}</template>
      </p>
      <div v-if="revoking" class="flex flex-wrap items-center gap-2">
        <p>Stop access for everyone using this link?</p>
        <Button
          label="Revoke access"
          type="button"
          severity="danger"
          :disabled="saving"
          @click="revoke"
        />
        <Button
          label="Cancel"
          type="button"
          severity="secondary"
          :disabled="saving"
          @click="revoking = false"
        />
      </div>
      <h2 class="m-0 text-lg font-semibold">
        {{
          complete
            ? 'All documents accepted'
            : ready === 1
              ? '1 item ready for review'
              : ready
                ? `${ready} items ready for review`
                : 'No items waiting for review'
        }}
      </h2>
      <ul
        class="divide-y divide-surface-200 rounded-xl border border-surface-200 bg-surface-0 dark:divide-surface-700 dark:border-surface-700 dark:bg-surface-900"
        aria-label="Requested documents"
      >
        <li v-for="item in request.items" :key="item.id" class="space-y-2 px-4 py-3">
          <div class="flex flex-wrap items-baseline justify-between gap-2">
            <h3 class="m-0 text-base leading-snug font-semibold wrap-anywhere">{{ item.name }}</h3>
            <span class="text-sm text-muted-color">{{ documentRequestLabels[item.status] }}</span>
          </div>
          <p
            v-if="item.instructions"
            class="text-sm text-muted-color wrap-anywhere whitespace-pre-wrap"
          >
            {{ item.instructions }}
          </p>
          <p v-if="item.status === 'replacement'" class="text-sm wrap-anywhere whitespace-pre-wrap">
            {{ item.reason }}
          </p>
          <ul
            v-if="item.files.some((file) => file.round === item.round)"
            class="space-y-1 text-sm"
            :aria-label="`Current files for ${item.name}`"
          >
            <li
              v-for="file in item.files.filter((file) => file.round === item.round)"
              :key="file.id"
            >
              <RouterLink
                :to="{ name: 'FileView', params: { fileId: file.id } }"
                class="text-primary underline wrap-anywhere"
                >{{ file.name }}</RouterLink
              >
            </li>
          </ul>
          <p
            v-if="
              item.status === 'submitted' && !item.files.some((file) => file.round === item.round)
            "
            class="text-sm text-muted-color"
          >
            The submitted files are no longer available. Ask for a replacement.
          </p>
          <div v-if="item.status === 'submitted'" class="space-y-2">
            <form
              v-if="returning === item.id"
              class="space-y-2"
              @submit.prevent="review(item, 'replacement')"
            >
              <label class="block space-y-1">
                <span>Reason to show the recipient</span>
                <textarea v-model="reason" :class="inputClass" maxlength="1000" rows="2" required />
              </label>
              <div class="flex flex-wrap gap-2">
                <Button
                  type="submit"
                  label="Request replacement"
                  :disabled="saving || !reason.trim()"
                />
                <Button
                  type="button"
                  label="Cancel"
                  severity="secondary"
                  :disabled="saving"
                  @click="returning = ''"
                />
              </div>
            </form>
            <div v-else class="flex flex-wrap gap-2">
              <Button
                type="button"
                label="Accept"
                :aria-label="`Accept ${item.name}`"
                :disabled="saving || !item.files.some((file) => file.round === item.round)"
                @click="review(item, 'accepted')"
              />
              <Button
                type="button"
                label="Ask for replacement"
                :aria-label="`Ask for replacement for ${item.name}`"
                severity="secondary"
                :disabled="saving"
                @click="
                  returning = item.id;
                  reason = '';
                "
              />
            </div>
          </div>
          <details v-if="item.history?.length" class="text-sm text-muted-color">
            <summary class="cursor-pointer">Review history</summary>
            <ul class="mt-2 space-y-2">
              <li v-for="review in item.history" :key="review.round">
                <p>
                  {{ documentRequestLabels[review.status] }} ·
                  {{ new Date(review.reviewed_at).toLocaleString() }}
                </p>
                <p v-if="review.reason" class="wrap-anywhere whitespace-pre-wrap">
                  {{ review.reason }}
                </p>
                <ul v-if="review.round !== item.round">
                  <li
                    v-for="file in item.files.filter((file) => file.round === review.round)"
                    :key="file.id"
                  >
                    <RouterLink
                      :to="{ name: 'FileView', params: { fileId: file.id } }"
                      class="text-primary underline wrap-anywhere"
                      >{{ file.name }}</RouterLink
                    >
                  </li>
                </ul>
              </li>
            </ul>
          </details>
        </li>
      </ul>
    </template>
  </section>
</template>
