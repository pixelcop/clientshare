<script setup lang="ts">
import Button from 'openvue/button';
import { computed, nextTick, ref } from 'vue';

const storageKey = 'document-request-prototype:october-close';
const items = [
  {
    id: 'bank',
    name: 'October bank statement',
    reason: 'Page 2 is missing. Please upload the full statement, including every page.',
  },
  { id: 'balance', name: 'September closing balance', reason: '' },
] as const;

type ItemId = (typeof items)[number]['id'];

function savedSubmissions(): ItemId[] {
  try {
    const saved: unknown = JSON.parse(localStorage.getItem(storageKey) ?? '[]');
    return items.map(({ id }) => id).filter((id) => Array.isArray(saved) && saved.includes(id));
  } catch {
    return [];
  }
}

const submitted = ref(savedSubmissions());
const selected = ref<Partial<Record<ItemId, string[]>>>({});
const statusHeading = ref<HTMLElement | null>(null);
const announcement = ref('');
const orderedItems = computed(() => [
  ...items.filter(({ id }) => !submitted.value.includes(id)),
  ...items.filter(({ id }) => submitted.value.includes(id)),
]);
const remaining = computed(() => items.length - submitted.value.length);

function selectFiles(id: ItemId, event: Event) {
  const input = event.target as HTMLInputElement;
  const names = Array.from(input.files ?? [], ({ name }) => name);
  if (!names.length) {
    return;
  }
  selected.value[id] = names;
  input.value = '';
}

async function submit(id: ItemId) {
  if (!selected.value[id]?.length || submitted.value.includes(id)) {
    return;
  }

  submitted.value = [...submitted.value, id];
  delete selected.value[id];
  announcement.value = `${items.find((item) => item.id === id)?.name} submitted for review.`;
  // Persist fixture state only. Do not read or retain real file contents or names.
  try {
    localStorage.setItem(storageKey, JSON.stringify(submitted.value));
  } catch {
    // The prototype still works for the current visit if storage is unavailable.
  }
  await nextTick();
  statusHeading.value?.focus();
}

async function reset() {
  submitted.value = [];
  selected.value = {};
  announcement.value = '';
  try {
    localStorage.removeItem(storageKey);
  } catch {
    // In-memory state has already been cleared.
  }
  await nextTick();
  statusHeading.value?.focus();
}
</script>

<template>
  <div class="min-h-dvh bg-slate-50 text-slate-950">
    <header class="border-b border-slate-200 bg-white">
      <div class="mx-auto flex max-w-3xl items-center justify-between gap-3 px-4 py-4 sm:px-8">
        <span class="text-base font-semibold">ClientShare</span>
        <span class="text-sm font-medium text-slate-600">Prototype</span>
      </div>
    </header>

    <main class="mx-auto max-w-3xl px-4 pt-7 pb-8 sm:px-8 sm:pt-12">
      <h1 class="text-3xl leading-tight font-semibold tracking-tight sm:text-4xl">October close</h1>
      <p class="mt-2 text-slate-600">Document request from Northline Accounting</p>

      <section
        aria-labelledby="request-status"
        class="mt-6 rounded-xl border border-slate-200 bg-white"
      >
        <div class="px-4 py-4 sm:px-6">
          <h2
            id="request-status"
            ref="statusHeading"
            tabindex="-1"
            class="text-lg font-semibold tracking-tight focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-sky-700"
          >
            <template v-if="remaining === 2">2 items need your attention</template>
            <template v-else-if="remaining === 1">1 item needs your attention</template>
            <template v-else>Nothing for you to do right now</template>
          </h2>
          <p v-if="remaining === 2" class="mt-1 text-sm text-slate-600">
            Replace the bank statement and add the missing closing balance.
          </p>
          <p v-else-if="remaining === 1" class="mt-1 text-sm text-slate-600">
            {{
              orderedItems[0].reason
                ? 'Replace the bank statement.'
                : 'Add the missing closing balance.'
            }}
          </p>
          <p v-else class="mt-1 text-sm text-slate-600">
            Northline Accounting is reviewing your documents.
          </p>
          <p role="status" class="sr-only">{{ announcement }}</p>
        </div>

        <ul
          class="divide-y divide-slate-200 border-t border-slate-200"
          aria-label="Requested documents"
        >
          <li v-for="item in orderedItems" :key="item.id" class="px-4 py-4 sm:px-6">
            <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
              <h3 class="font-semibold">{{ item.name }}</h3>
              <span v-if="submitted.includes(item.id)" class="text-sm font-medium text-sky-800">
                Submitted for review
              </span>
              <span v-else-if="item.reason" class="text-sm font-semibold text-amber-900">
                Replacement needed
              </span>
              <span v-else class="text-sm font-medium text-slate-600">Not submitted</span>
            </div>

            <template v-if="!submitted.includes(item.id)">
              <p
                v-if="item.reason"
                :id="`${item.id}-reason`"
                class="mt-3 rounded-lg bg-amber-50 px-3 py-2.5 text-sm leading-relaxed text-amber-950"
              >
                {{ item.reason }}
              </p>
              <p v-else class="mt-1 text-sm text-slate-600">
                Include the balance as of September 30.
              </p>

              <ul
                v-if="selected[item.id]?.length"
                :aria-label="`Selected files for ${item.name}`"
                class="mt-3 space-y-1 text-sm text-slate-700"
              >
                <li
                  v-for="(name, index) in selected[item.id]"
                  :key="index"
                  class="rounded-lg bg-slate-50 px-3 py-2 wrap-anywhere"
                >
                  {{ name }}
                </li>
              </ul>

              <div class="mt-3 flex flex-wrap items-center gap-2">
                <label class="inline-flex cursor-pointer items-center">
                  <input
                    type="file"
                    multiple
                    class="peer sr-only"
                    :aria-label="`Choose ${item.reason ? 'replacement files' : 'files'} for ${item.name}`"
                    :aria-describedby="item.reason ? `${item.id}-reason` : undefined"
                    @change="selectFiles(item.id, $event)"
                  />
                  <span
                    class="inline-flex min-h-11 items-center rounded-md border border-sky-700 px-3 text-sm font-semibold text-sky-800 hover:bg-sky-50 peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-sky-700"
                  >
                    {{
                      selected[item.id]?.length
                        ? 'Change files'
                        : item.reason
                          ? 'Add replacement'
                          : 'Add files'
                    }}
                  </span>
                </label>
                <Button
                  v-if="selected[item.id]?.length"
                  type="button"
                  label="Submit for review"
                  :aria-label="`Submit ${item.name} for review`"
                  class="inline-flex min-h-11 cursor-pointer items-center rounded-md bg-sky-800 px-3 text-sm font-semibold text-white hover:bg-sky-900 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-sky-700"
                  @click="submit(item.id)"
                />
              </div>
            </template>
          </li>
          <li class="px-4 py-4 sm:px-6">
            <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
              <h3 class="font-semibold">October payroll summary</h3>
              <span class="text-sm font-medium text-emerald-800">Accepted</span>
            </div>
          </li>
        </ul>
      </section>

      <footer
        class="mt-3 flex flex-wrap items-center justify-between gap-x-4 text-sm text-slate-600"
      >
        <p>Demo only. No files are uploaded.</p>
        <button
          type="button"
          class="min-h-11 cursor-pointer rounded-sm font-medium text-slate-700 underline decoration-slate-400 underline-offset-4 hover:text-sky-800 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-sky-700"
          @click="reset"
        >
          Reset demo
        </button>
      </footer>
    </main>
  </div>
</template>
