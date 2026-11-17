<script setup lang="ts">
import axios from 'axios';
import { Button, Textarea } from 'openvue';
import { nextTick, onBeforeUnmount, ref, watch } from 'vue';

import CommentBody from '@/components/CommentBody.vue';

const props = defineProps<{
  id: string;
  fileId: string;
  sessionKey: string;
  visibility: string;
  disabled?: boolean;
  readonly?: boolean;
  label?: string;
  rows?: number;
}>();
const body = defineModel<string>({ required: true });
interface Person {
  id: string;
  name: string;
  email?: string;
}
const preview = ref(false);
const html = ref('');
const busy = ref(false);
const error = ref('');
const people = ref<Person[]>([]);
const selected = ref(0);
const searching = ref(false);
const mentionError = ref('');
let range: { start: number; end: number; query: string } | null = null;
let timer: ReturnType<typeof setTimeout> | undefined;
let generation = 0;
let mentionGeneration = 0;
let controller: AbortController | undefined;
let mentionController: AbortController | undefined;

function clearMentions() {
  clearTimeout(timer);
  mentionGeneration++;
  mentionController?.abort();
  people.value = [];
  searching.value = false;
  mentionError.value = '';
  range = null;
}
function invalidate() {
  generation++;
  controller?.abort();
  preview.value = false;
  html.value = '';
  error.value = '';
  busy.value = false;
  clearMentions();
}
watch(
  () => [props.fileId, props.sessionKey, props.visibility, props.readonly, props.disabled],
  invalidate,
);
watch(
  body,
  () => {
    generation++;
    controller?.abort();
    html.value = '';
    busy.value = false;
    clearMentions();
  },
  { flush: 'sync' },
);
onBeforeUnmount(invalidate);

async function showPreview() {
  clearMentions();
  preview.value = true;
  html.value = '';
  error.value = '';
  if (!body.value.trim()) {
    return;
  }
  const current = ++generation;
  controller?.abort();
  controller = new AbortController();
  busy.value = true;
  try {
    const { data } = await axios.post<{ body_html: string }>(
      `/api/files/${props.fileId}/comments/preview`,
      { body: body.value, visibility: props.visibility },
      { signal: controller.signal },
    );
    if (current === generation) {
      html.value = data.body_html;
    }
  } catch (cause) {
    if (current === generation && !axios.isCancel(cause)) {
      error.value = 'Preview unavailable. Turn off Preview to keep editing, or try again.';
    }
  } finally {
    if (current === generation) {
      busy.value = false;
    }
  }
}
async function write() {
  invalidate();
  await nextTick();
  document.getElementById(props.id)?.focus();
}
function findMention(event: Event) {
  clearMentions();
  if (
    preview.value ||
    props.readonly ||
    props.disabled ||
    (event instanceof InputEvent && event.isComposing)
  ) {
    return;
  }
  const input = event.target as HTMLTextAreaElement;
  if (input.selectionStart !== input.selectionEnd) {
    return;
  }
  const end = input.selectionStart;
  const match = /(?:^|[\s(])@([^\s@()[\]`]{1,80})$/u.exec(input.value.slice(0, end));
  if (!match) {
    return;
  }
  range = { start: end - match[1]!.length - 1, end, query: match[1]! };
  const current = mentionGeneration;
  const query = range.query;
  searching.value = true;
  timer = setTimeout(async () => {
    mentionController = new AbortController();
    try {
      const { data } = await axios.get<Person[]>(`/api/files/${props.fileId}/comments/mentions`, {
        params: { search: query, visibility: props.visibility },
        signal: mentionController.signal,
      });
      if (current === mentionGeneration) {
        people.value = data;
        selected.value = 0;
        mentionError.value = data.length ? '' : 'No people available for this mention.';
      }
    } catch (cause) {
      if (current === mentionGeneration && !axios.isCancel(cause)) {
        mentionError.value = 'Mention search unavailable. Try typing the name again.';
      }
    } finally {
      if (current === mentionGeneration) {
        searching.value = false;
      }
    }
  }, 200);
}
async function choose(person: Person) {
  if (!range) {
    return;
  }
  const label = person.name.replace(/[\\`*_[\]<>]/g, '\\$&').replace(/[\r\n]/g, ' ');
  const insertion = `[@${label}](mention:${person.id}) `;
  const cursor = range.start + insertion.length;
  body.value = body.value.slice(0, range.start) + insertion + body.value.slice(range.end);
  clearMentions();
  await nextTick();
  const input = document.getElementById(props.id) as HTMLTextAreaElement | null;
  input?.focus();
  input?.setSelectionRange(cursor, cursor);
}
function keydown(event: KeyboardEvent) {
  if (!range || event.isComposing) {
    return;
  }
  if (event.key === 'Escape') {
    event.preventDefault();
    event.stopPropagation();
    clearMentions();
  } else if (people.value.length && ['ArrowDown', 'ArrowUp', 'Enter'].includes(event.key)) {
    event.preventDefault();
    event.stopPropagation();
    if (event.key === 'Enter') {
      void choose(people.value[selected.value]!);
    } else {
      selected.value =
        (selected.value + (event.key === 'ArrowDown' ? 1 : -1) + people.value.length) %
        people.value.length;
    }
  }
}
</script>

<template>
  <div class="markdown-editor">
    <div class="editor-heading">
      <label :for="id">{{ label || 'Comment' }}</label>
      <Button
        type="button"
        class="preview-toggle"
        label="Preview"
        icon="oi oi-eye"
        size="small"
        severity="secondary"
        variant="text"
        :title="preview ? 'Return to editing' : 'Preview comment'"
        :aria-pressed="preview"
        :aria-controls="preview ? `${id}-preview` : id"
        :disabled="disabled || readonly || Array.from(body).length > 4000"
        @click="preview ? write() : showPreview()"
      />
    </div>
    <div
      v-if="preview"
      :id="`${id}-preview`"
      class="markdown-preview"
      aria-label="Comment preview"
      :aria-busy="busy"
      role="region"
      tabindex="0"
    >
      <p v-if="busy" role="status">Loading preview…</p>
      <p v-else-if="error" role="alert">{{ error }}</p>
      <p v-else-if="!body.trim()">Nothing to preview.</p>
      <CommentBody v-else :body="body" :html="html" />
    </div>
    <Textarea
      v-else
      :id="id"
      v-model="body"
      :rows="rows || 3"
      fluid
      :maxlength="8000"
      :disabled="disabled"
      :readonly="readonly"
      :invalid="Array.from(body).length > 4000"
      :aria-describedby="`${id}-length`"
      :aria-controls="people.length ? `${id}-mentions` : undefined"
      :aria-activedescendant="people.length ? `${id}-mention-${selected}` : undefined"
      aria-autocomplete="list"
      placeholder="Write Markdown · type @name to mention"
      @input="findMention"
      @click="findMention"
      @keyup.left="findMention"
      @keyup.right="findMention"
      @keydown="keydown"
      @blur="clearMentions"
    />
    <small :id="`${id}-length`" class="editor-length">{{ Array.from(body).length }}/4000</small>
    <ul
      v-if="people.length"
      :id="`${id}-mentions`"
      class="mention-options"
      role="listbox"
      aria-label="Mention suggestions"
    >
      <li
        v-for="(person, index) in people"
        :id="`${id}-mention-${index}`"
        :key="person.id"
        role="option"
        :aria-selected="index === selected"
      >
        <button type="button" @mousedown.prevent @click="choose(person)">
          <strong>@{{ person.name }}</strong
          ><small v-if="person.email">{{ person.email }}</small>
        </button>
      </li>
    </ul>
    <small v-if="searching || mentionError" role="status">{{
      searching ? 'Finding people…' : mentionError
    }}</small>
  </div>
</template>

<style scoped>
.markdown-editor {
  min-width: 0;
}
.editor-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 8px;
}
.editor-heading label {
  font-size: 14px;
  font-weight: 500;
}
.preview-toggle {
  flex-shrink: 0;
  min-height: 40px;
}
.preview-toggle[aria-pressed='true'] {
  background: var(--p-surface-100);
}
.editor-length {
  display: block;
  margin-top: 4px;
  text-align: end;
  font-variant-numeric: tabular-nums;
}
.mention-options button:focus-visible {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 2px;
}
.markdown-preview {
  min-height: 100px;
  max-height: 180px;
  overflow: auto;
  padding: 10px;
  border: 1px solid var(--p-content-border-color);
  border-radius: var(--p-border-radius-sm);
}
.markdown-preview p {
  margin: 0;
  font-size: 13px;
}
textarea {
  resize: vertical;
  max-height: 180px;
}
.mention-options {
  list-style: none;
  padding: 4px;
  margin: 4px 0;
  max-height: 160px;
  overflow: auto;
  border: 1px solid var(--p-content-border-color);
  border-radius: var(--p-border-radius-sm);
}
.mention-options button {
  display: grid;
  width: 100%;
  text-align: start;
  font: inherit;
  font-size: 13px;
  padding: 8px;
  min-height: 40px;
  border: 0;
  background: transparent;
  color: var(--p-text-color);
  overflow-wrap: anywhere;
  cursor: pointer;
}
.mention-options [aria-selected='true'] button,
.mention-options button:hover {
  background: var(--p-surface-100);
}
small {
  color: var(--p-text-muted-color);
  font-size: 12px;
}
</style>
