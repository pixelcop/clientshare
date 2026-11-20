<script setup lang="ts">
import axios from 'axios';
import Button from 'openvue/button';
import { computed, nextTick, ref, watch } from 'vue';
import { useRouter } from 'vue-router';

import DocumentRequestGuidance from '@/components/DocumentRequestGuidance.vue';
import RecipientPicker from '@/components/RecipientPicker.vue';
import {
  type DocumentRequest,
  type DocumentRequestItem,
  documentRequestLabels,
  type DocumentRequestSummary,
  requestFileRoute,
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
const adding = ref(false);
const description = ref('');
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
const newRow = () => ({ key: nextRow++, name: '', instructions: '', showInstructions: false });
const draft = ref([newRow()]);

function beginAddition() {
  draft.value = [newRow()];
  description.value = '';
  notice.value = '';
  error.value = '';
  adding.value = true;
}

async function revealInstructions(item: ReturnType<typeof newRow>) {
  item.showInstructions = true;
  await nextTick();
  document.getElementById(`request-item-instructions-${item.key}`)?.focus();
}
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
    adding.value = false;
    description.value = '';
    draft.value = [newRow()];
    editingFolder.value = false;
    void load();
  },
  { immediate: true },
);

async function create() {
  const append = adding.value;
  if (
    saving.value ||
    (!append && (editingFolder.value || (!emailAll.value && recipientEmail.value.trim())))
  ) {
    return;
  }
  saving.value = true;
  error.value = '';
  try {
    const { data } = await axios.post<DocumentRequest>(
      append ? `${detail.value}/additions` : endpoint.value,
      {
        ...(append
          ? {}
          : {
              title: title.value,
              folder_name: folderName.value,
              email_all: emailAll.value,
              recipient_emails: emailAll.value ? [] : recipientEmails.value,
            }),
        description: description.value,
        items: draft.value.map(({ name, instructions }) => ({ name, instructions })),
      },
    );
    title.value = '';
    recipientEmail.value = '';
    recipientEmails.value = [];
    emailAll.value = true;
    folderOverride.value = null;
    draft.value = [newRow()];
    description.value = '';
    if (append) {
      request.value = data;
      adding.value = false;
    } else {
      await router.push(requestRoute(data.id));
    }
    const saved = append ? 'Documents added.' : 'Request created.';
    notice.value = saved;
    if (data.email_status === 'queued') {
      const count = data.email_count ?? 0;
      notice.value = `${saved} Email queued for ${count === 1 ? '1 client user' : `${count} client users`}.`;
    } else if (data.email_status === 'unavailable') {
      notice.value = `${saved} Email is unavailable; copy and share the request link.`;
    } else if (data.email_status === 'no_recipients') {
      notice.value = `${saved} No client users could be emailed; copy and share the request link.`;
    }
  } catch (failure) {
    error.value =
      (axios.isAxiosError<{ error?: string }>(failure) && failure.response?.data?.error) ||
      (append
        ? 'Could not add documents. Try again.'
        : 'Could not create the request. Check the title and item names, then try again.');
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
      :to="requestId || creating ? requestRoute() : { name: 'Clients' }"
      class="inline-block text-sm text-primary hover:underline"
      @click="creating = false"
    >
      {{ requestId || creating ? 'All document requests' : 'All clients' }}
    </RouterLink>
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h1 class="m-0 text-2xl font-semibold tracking-tight wrap-anywhere">
        {{ creating ? 'New document request' : (request?.title ?? 'Document requests') }}
      </h1>
      <Button
        v-if="!requestId && !creating && !loading && !error"
        type="button"
        label="New request"
        @click="creating = true"
      />
      <Button
        v-if="request?.link && !adding"
        type="button"
        label="Request more documents"
        icon="oi oi-plus"
        severity="secondary"
        variant="outlined"
        @click="beginAddition"
      />
    </div>

    <p v-if="loading" role="status">Loading document requests…</p>
    <p v-if="error" role="alert" class="text-red-700 dark:text-red-300">{{ error }}</p>
    <p v-if="notice" role="status">{{ notice }}</p>

    <form
      v-if="(!requestId && creating) || adding"
      class="rounded-xl border border-surface-200 bg-surface-0 dark:border-surface-700 dark:bg-surface-900"
      @submit.prevent="create"
    >
      <div v-if="!adding" class="px-4 pt-4 sm:px-6 sm:pt-6">
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
            <span class="font-medium wrap-anywhere">{{
              folderName || 'Same as request title'
            }}</span>
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
      </div>
      <div class="px-4 pt-4 sm:px-6">
        <label class="block font-medium" for="request-description">
          {{ adding ? 'Message for these additional documents' : 'Request description' }}
          <span class="font-normal text-muted-color">(optional)</span>
        </label>
        <textarea
          id="request-description"
          v-model="description"
          :class="inputClass"
          class="mt-1"
          rows="3"
          maxlength="4000"
          aria-describedby="request-description-help"
        />
        <p id="request-description-help" class="m-0 text-sm text-muted-color">
          Shown above the checklist and included in the email if one is sent.
        </p>
      </div>
      <section aria-labelledby="request-documents-heading" class="px-4 pt-4 pb-5 sm:px-6">
        <h2 id="request-documents-heading" class="m-0 text-lg font-semibold">
          {{ adding ? 'Request more documents' : 'Documents' }}
        </h2>
        <ol class="mt-2 divide-y divide-surface-200 dark:divide-surface-700">
          <li v-for="(item, index) in draft" :key="item.key" class="py-3">
            <fieldset class="min-w-0">
              <legend class="sr-only">Document {{ index + 1 }}</legend>
              <div class="flex items-end gap-2">
                <label :for="`request-item-name-${item.key}`" class="flex-1 font-medium"
                  >Document {{ index + 1 }}</label
                >
                <Button
                  v-if="draft.length > 1"
                  type="button"
                  label="Remove"
                  severity="secondary"
                  variant="text"
                  size="small"
                  :aria-label="`Remove document ${index + 1}`"
                  @click="draft.splice(index, 1)"
                />
              </div>
              <div>
                <input
                  :id="`request-item-name-${item.key}`"
                  v-model="item.name"
                  :class="inputClass"
                  class="mt-1"
                  maxlength="160"
                  required
                />
                <button
                  v-if="!item.showInstructions"
                  type="button"
                  class="min-h-8 cursor-pointer rounded-sm text-sm text-primary-emphasis underline underline-offset-2 focus-visible:outline-2 focus-visible:outline-primary"
                  @click="revealInstructions(item)"
                >
                  Add instructions
                </button>
                <label v-else class="mt-2 block space-y-1">
                  <span>Instructions <span class="text-muted-color">(optional)</span></span>
                  <textarea
                    :id="`request-item-instructions-${item.key}`"
                    v-model="item.instructions"
                    :class="inputClass"
                    maxlength="1000"
                    rows="2"
                  />
                </label>
              </div>
            </fieldset>
          </li>
        </ol>
        <Button
          type="button"
          label="Add document"
          icon="oi oi-plus"
          severity="secondary"
          variant="outlined"
          :disabled="draft.length >= 30"
          @click="draft.push(newRow())"
        />
      </section>
      <div
        class="space-y-4 rounded-b-xl border-t border-surface-200 bg-surface-50 px-4 py-4 sm:px-6 dark:border-surface-700 dark:bg-surface-800"
      >
        <fieldset v-if="!adding" class="min-w-0 space-y-1">
          <legend class="text-base font-semibold">Delivery</legend>
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
        <p v-else class="m-0 text-sm text-muted-color">
          The same link will work. The original recipients will be notified if they still have
          access.
        </p>
        <div class="flex flex-wrap gap-2">
          <Button
            type="submit"
            :label="adding ? 'Add to request' : 'Create request'"
            :disabled="
              saving || (!adding && (editingFolder || (!emailAll && !!recipientEmail.trim())))
            "
          />
          <Button
            type="button"
            label="Cancel"
            severity="secondary"
            variant="text"
            :disabled="saving"
            @click="adding ? (adding = false) : (creating = false)"
          />
        </div>
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
      <DocumentRequestGuidance :request />
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
            v-if="item.files.length"
            class="space-y-1 text-sm"
            :aria-label="`Files for ${item.name}`"
          >
            <li v-for="file in item.files" :key="file.id">
              <div class="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                <RouterLink
                  :to="requestFileRoute(clientId, request.id, file.id!)"
                  class="text-primary underline wrap-anywhere"
                >
                  {{ file.name }}
                </RouterLink>
                <span v-if="file.comment_count" class="inline-flex items-baseline gap-2">
                  <span aria-hidden="true" class="text-muted-color">·</span>
                  <RouterLink
                    :to="requestFileRoute(clientId, request.id, file.id!, true)"
                    :aria-label="`${file.comment_count} ${file.comment_count === 1 ? 'comment' : 'comments'} on ${file.name}`"
                    class="text-primary underline"
                  >
                    {{ file.comment_count }} {{ file.comment_count === 1 ? 'comment' : 'comments' }}
                  </RouterLink>
                </span>
              </div>
              <time v-if="file.submitted_at" :datetime="file.submitted_at" class="text-muted-color">
                Submitted {{ new Date(file.submitted_at).toLocaleString() }}
              </time>
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
              </li>
            </ul>
          </details>
        </li>
      </ul>
    </template>
  </section>
</template>
