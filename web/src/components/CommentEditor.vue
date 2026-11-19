<script setup lang="ts">
import { shift } from '@floating-ui/dom';
import { PluginKey } from '@tiptap/pm/state';
import { exitSuggestion, type SuggestionProps } from '@tiptap/suggestion';
import { EditorContent, useEditor } from '@tiptap/vue-3';
import { useResizeObserver } from '@vueuse/core';
import { Popover } from 'openvue';
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, watch } from 'vue';

import { type MentionSearch, useMentionSearch } from '@/composables/useMentionSearch';
import type { SharingRecipient } from '@/types/sharing';
import {
  commentExtensions,
  type CommentMentionAttrs,
  type CommentPerson,
} from '@/utils/commentEditor';

const props = defineProps<{
  id: string;
  fileId: string;
  sessionKey: string;
  visibility: string;
  disabled?: boolean;
  readonly?: boolean;
  label?: string;
  rows?: number;
  mentionSearch?: MentionSearch;
  mentionHTML?: string | null;
  shareMention?: (person: SharingRecipient) => Promise<boolean>;
  unavailableMentionIds?: string[];
}>();
const emit = defineEmits<{ 'mentions-converted': [] }>();
const mentionNames = computed(() => {
  const fragment = document.createElement('template');
  fragment.innerHTML = props.mentionHTML || '';
  return new Map(
    Array.from(
      fragment.content.querySelectorAll<HTMLElement>('.comment-mention[data-user-id]'),
      (node) => [node.dataset.userId, node.textContent?.slice(1)],
    ),
  );
});
const mentions = props.mentionSearch ?? useMentionSearch(() => props);
const body = defineModel<string>({ required: true });
const mentionKey = new PluginKey(`comment-mentions-${props.id}`);
const suggestion = shallowRef<SuggestionProps<CommentPerson, CommentMentionAttrs> | null>(null);
const keyboardId = ref<string | null>(null);
const hoveredId = ref<string | null>(null);
const selected = computed(() =>
  Math.max(0, suggestion.value?.items.findIndex((person) => person.id === keyboardId.value) ?? -1),
);
const popup = ref<HTMLUListElement | null>(null);
const frame = ref<HTMLDivElement | null>(null);
const markdownHelp = ref<InstanceType<typeof Popover> | null>(null);
const mentionError = ref('');
const empty = ref(!body.value.trim());
const length = computed(() => Array.from(body.value).length);
let popupGeneration = 0;
let unmountPopup: (() => void) | undefined;
let emittedSource = body.value;
let lastPointer: { x: number; y: number } | undefined;

function closePopup() {
  popupGeneration++;
  unmountPopup?.();
  unmountPopup = undefined;
  suggestion.value = null;
  keyboardId.value = null;
  hoveredId.value = null;
  lastPointer = undefined;
  mentionError.value = '';
}
function updateSuggestions(context: SuggestionProps<CommentPerson, CommentMentionAttrs>) {
  const previous = suggestion.value;
  const queryChanged = context.query !== previous?.query;
  if (queryChanged) {
    mentionError.value = '';
  }
  const cached = context.loading ? mentions.peek(props.visibility, context.query) : undefined;
  const items = context.loading
    ? (cached ?? mentions.local(props.visibility, context.query))
    : context.items;
  if (queryChanged || !items.some((person) => person.id === keyboardId.value)) {
    keyboardId.value = null;
  }
  if (
    queryChanged ||
    items.length !== previous?.items.length ||
    items.some((person, index) => person.id !== previous.items[index]?.id)
  ) {
    hoveredId.value = null;
  }
  suggestion.value = { ...context, items, loading: context.loading && !cached };
}
function hoverPerson(event: PointerEvent, id: string) {
  const { clientX: x, clientY: y } = event;
  if (event.pointerType === 'touch' || (lastPointer?.x === x && lastPointer.y === y)) {
    return;
  }
  lastPointer = { x, y };
  hoveredId.value = id;
}
async function openPopup(context: SuggestionProps<CommentPerson, CommentMentionAttrs>) {
  updateSuggestions(context);
  const current = ++popupGeneration;
  await nextTick();
  if (current === popupGeneration && popup.value) {
    const element = popup.value;
    element.style.visibility = 'hidden';
    unmountPopup = context.mount(element, {
      onPosition: ({ x, y, strategy }) => {
        if (current === popupGeneration) {
          Object.assign(element.style, {
            position: strategy,
            left: `${x}px`,
            top: `${y}px`,
            visibility: '',
          });
        }
      },
    });
  }
}
async function choose(person: CommentPerson) {
  const active = suggestion.value;
  if (!active) {
    return;
  }
  const attrs = { id: person.id, label: person.name };
  if (person.action !== 'share') {
    active.command(attrs);
    return;
  }
  const current = active.editor;
  if (!props.shareMention || !person.email) {
    return;
  }
  const { doc, selection } = current.state;
  const { fileId, sessionKey, visibility } = props;
  exitSuggestion(current.view, mentionKey);
  const granted = await props.shareMention({
    id: person.id,
    name: person.name,
    email: person.email,
    userType: person.user_type,
  });
  if (
    current.isDestroyed ||
    props.fileId !== fileId ||
    props.sessionKey !== sessionKey ||
    props.visibility !== visibility ||
    props.disabled ||
    props.readonly
  ) {
    return;
  }
  if (granted && current.state.doc.eq(doc) && current.state.selection.eq(selection)) {
    active.command(attrs);
  } else {
    current.commands.focus();
  }
}
const mentionDetail = {
  share: 'No file access · Share file…',
  reference_team: 'Client · Can’t see team notes',
  reference_no_access: 'Internal · No access to this file',
};
function editorAttributes() {
  return {
    id: props.id,
    role: 'textbox',
    'aria-label': props.label || 'Comment',
    'aria-multiline': 'true',
    'aria-readonly': String(!!props.readonly),
    'aria-disabled': String(!!props.disabled),
    'aria-invalid': String(length.value > 4000),
    'aria-describedby': length.value > 4000 ? `${props.id}-limit` : '',
    'aria-autocomplete': 'list',
    'aria-controls': suggestion.value ? `${props.id}-mentions` : '',
    'aria-activedescendant': suggestion.value?.items.length
      ? `${props.id}-mention-${selected.value}`
      : '',
  };
}
const editor = useEditor({
  content: body.value,
  contentType: 'markdown',
  editable: !props.disabled && !props.readonly,
  autofocus: 'end',
  extensions: commentExtensions(
    {
      pluginKey: mentionKey,
      char: '@',
      allowedPrefixes: [' ', '(', '\n'],
      debounce: 200,
      allow: ({ editor: current }) =>
        !props.disabled &&
        !props.readonly &&
        !current.isActive('code') &&
        !current.isActive('codeBlock'),
      floatingUi: { strategy: 'fixed', middleware: [shift({ padding: 12 })] },
      items: async ({ query, signal }) => {
        if (signal.aborted || props.disabled || props.readonly) {
          return [];
        }
        const visibility = props.visibility;
        try {
          const data = await mentions.search(visibility, query);
          return signal.aborted ? [] : data;
        } catch {
          if (!signal.aborted) {
            mentionError.value = 'Mention search unavailable. Try typing the name again.';
            return mentions.local(visibility, query);
          }
          return [];
        }
      },
      render: () => ({
        onStart: (context) => {
          void openPopup(context);
        },
        onUpdate: updateSuggestions,
        onKeyDown: ({ event, view }) => {
          if (event.isComposing) {
            return false;
          }
          if (event.key === 'Escape') {
            event.preventDefault();
            event.stopPropagation();
            exitSuggestion(view, mentionKey);
            return true;
          }
          const people = suggestion.value?.items || [];
          if (!people.length || !['ArrowDown', 'ArrowUp', 'Enter'].includes(event.key)) {
            return false;
          }
          event.preventDefault();
          event.stopPropagation();
          if (event.key === 'Enter') {
            choose(people[selected.value]!);
          } else {
            const index =
              (selected.value + (event.key === 'ArrowDown' ? 1 : -1) + people.length) %
              people.length;
            keyboardId.value = people[index]!.id;
            hoveredId.value = null;
            void nextTick(() => {
              document
                .getElementById(`${props.id}-mention-${selected.value}`)
                ?.scrollIntoView?.({ block: 'nearest' });
            });
          }
          return true;
        },
        onExit: closePopup,
      }),
    },
    {
      displayName: (id) => mentionNames.value.get(id),
      unavailableIds: () => props.unavailableMentionIds,
    },
  ),
  editorProps: {
    attributes: editorAttributes,
    clipboardTextSerializer: (slice) =>
      editor.value?.markdown?.serialize({
        type: 'doc',
        content: slice.content.toJSON() || [],
      }) || '',
    handlePaste: (_view, event) => {
      const text = event.clipboardData?.getData('text/plain');
      if (!text) {
        // Comments do not accept HTML-only clipboard payloads or remote images.
        return !!event.clipboardData?.getData('text/html');
      }
      editor.value?.commands.insertContent(text, { contentType: 'markdown' });
      return true;
    },
  },
  onUpdate: ({ editor: current }) => {
    empty.value = current.isEmpty;
    emittedSource = current.isEmpty ? '' : current.getMarkdown();
    body.value = emittedSource;
  },
});

function convertUnavailableMentions() {
  const unavailable = new Set(props.unavailableMentionIds);
  editor.value?.commands.command(({ state, tr }) => {
    // Work from the end of the document so earlier positions stay valid.
    const replacements: { from: number; to: number; text: string }[] = [];
    state.doc.descendants((node, pos) => {
      if (node.type.name === 'mention' && unavailable.has(node.attrs.id)) {
        replacements.push({
          from: pos,
          to: pos + node.nodeSize,
          text: `@${mentionNames.value.get(node.attrs.id) || node.attrs.label || node.attrs.id}`,
        });
      }
    });
    replacements.reverse().forEach(({ from, to, text }) => tr.insertText(text, from, to));
    return !!replacements.length;
  });
  emit('mentions-converted');
  editor.value?.commands.focus();
}

// Grow to fit the rendered document, not the serialized Markdown. Never shrink
// a height the reader has already chosen with the resize handle.
useResizeObserver(
  computed(() => editor.value?.view.dom),
  () => {
    const container = frame.value;
    const content = editor.value?.view.dom;
    if (!container?.offsetHeight || !content) {
      return;
    }
    const maxHeight = Number.parseFloat(getComputedStyle(container).maxHeight) || Infinity;
    const border = container.offsetHeight - container.clientHeight;
    const height = Math.min(content.scrollHeight + border, maxHeight);
    if (height > container.offsetHeight) {
      container.style.height = `${height}px`;
    }
  },
);

watch(body, (source) => {
  // Echoed input must not replace the document or move the caret. Opening an
  // existing comment does not rewrite its source; edits serialize to Markdown.
  if (source !== emittedSource && editor.value) {
    exitSuggestion(editor.value.view, mentionKey);
    editor.value.commands.setContent(source, { contentType: 'markdown', emitUpdate: false });
    emittedSource = source;
    empty.value = editor.value.isEmpty;
  }
});
watch(
  () => [props.fileId, props.sessionKey, props.visibility, props.disabled, props.readonly],
  () => {
    if (editor.value) {
      exitSuggestion(editor.value.view, mentionKey);
      editor.value.setEditable(!props.disabled && !props.readonly, false);
    }
    closePopup();
  },
);
watch(
  [suggestion, selected, () => props.label, length, () => props.unavailableMentionIds],
  () => {
    if (editor.value) {
      editor.value.setOptions({
        editorProps: { ...editor.value.options.editorProps, attributes: editorAttributes },
      });
    }
  },
  { flush: 'post' },
);
onBeforeUnmount(closePopup);
</script>

<template>
  <div class="markdown-editor">
    <div :id="`${id}-heading`" class="editor-heading">{{ label || 'Comment' }}</div>
    <div class="editor-surface">
      <div
        ref="frame"
        class="editor-frame"
        :class="{ 'editor-tall': rows === 4, invalid: length > 4000, disabled }"
      >
        <EditorContent :editor="editor" class="rich-content" />
        <span v-if="empty" class="editor-placeholder" aria-hidden="true">Write a comment</span>
      </div>
      <ul
        v-if="suggestion"
        ref="popup"
        :id="`${id}-mentions`"
        class="mention-options"
        role="listbox"
        aria-label="Mention suggestions"
        data-sentry-mask
      >
        <li
          v-for="(person, index) in suggestion.items"
          :id="`${id}-mention-${index}`"
          :key="person.id"
          role="option"
          :aria-selected="index === selected"
          :class="{ 'mention-hovered': person.id === hoveredId }"
          @pointermove="hoverPerson($event, person.id)"
          @pointerleave="hoveredId = null"
        >
          <button type="button" tabindex="-1" @mousedown.prevent @click="choose(person)">
            <span class="mention-person">
              <strong>@{{ person.name }}</strong>
              <span v-if="person.email" class="mention-email">{{ person.email }}</span>
            </span>
            <small v-if="person.action" class="mention-detail">{{
              mentionDetail[person.action]
            }}</small>
          </button>
        </li>
        <li v-if="!suggestion.items.length" class="mention-state" role="presentation">
          <span role="status">{{
            suggestion.loading
              ? 'Finding people…'
              : mentionError || 'No people available for this mention.'
          }}</span>
        </li>
      </ul>
    </div>
    <p v-if="length > 4000" :id="`${id}-limit`" class="editor-error" role="alert">
      Comment is too long. The limit is 4,000 characters, including Markdown.
    </p>
    <div v-if="unavailableMentionIds?.length" class="editor-error">
      <p role="alert">
        A mentioned person is no longer available for this audience. Your draft was kept.
      </p>
      <button type="button" class="mention-recovery" @click="convertUnavailableMentions">
        Keep unavailable mentions as text
      </button>
    </div>
    <div class="editor-meta">
      <slot name="metadata" />
      <button type="button" class="markdown-help-trigger" @click="markdownHelp?.toggle($event)">
        Markdown supported
      </button>
      <Popover ref="markdownHelp" aria-label="Markdown and @mention quick reference">
        <dl class="markdown-help">
          <dt>Bold</dt>
          <dd><code>**text**</code></dd>
          <dt>Italic</dt>
          <dd><code>*text*</code></dd>
          <dt>Link</dt>
          <dd><code>[text](https://…)</code></dd>
          <dt>Mention</dt>
          <dd>Type <code>@</code> and choose a name.</dd>
        </dl>
      </Popover>
    </div>
  </div>
</template>

<style scoped>
.markdown-editor {
  min-width: 0;
}
.editor-heading {
  font-size: 14px;
  font-weight: 500;
  margin-bottom: 8px;
}
.editor-surface {
  position: relative;
  min-width: 0;
}
.editor-frame {
  box-sizing: border-box;
  position: relative;
  height: 80px;
  min-height: 80px;
  max-height: 240px;
  overflow: auto;
  resize: vertical;
  border: 1px solid var(--p-form-field-border-color);
  border-radius: var(--p-border-radius-sm);
  background: var(--p-form-field-background);
  color: var(--p-form-field-color);
}
.editor-tall {
  height: 100px;
}
.editor-frame:focus-within {
  border-color: var(--p-primary-color);
}
.editor-frame.invalid {
  border-color: var(--p-form-field-invalid-border-color);
}
.editor-frame.disabled {
  background: var(--p-form-field-disabled-background);
}
.editor-placeholder {
  position: absolute;
  top: 10px;
  inset-inline-start: 10px;
  inset-inline-end: 10px;
  pointer-events: none;
  font-size: 13px;
  color: var(--p-text-muted-color);
}
.rich-content {
  height: 100%;
}
.editor-frame :deep(.tiptap) {
  box-sizing: border-box;
  min-height: 100%;
  padding: 10px;
  outline: none;
  font-size: 14px;
  line-height: 1.6;
  overflow-wrap: anywhere;
}
.editor-frame :deep(p) {
  margin: 0 0 0.5em;
}
.editor-frame :deep(:last-child) {
  margin-bottom: 0;
}
.editor-frame :deep(ul) {
  list-style: disc;
  padding-inline-start: 1.5em;
}
.editor-frame :deep(ol) {
  list-style: decimal;
  padding-inline-start: 1.5em;
}
.editor-frame :deep(pre) {
  background: var(--p-surface-100);
  padding: 8px;
  overflow-x: auto;
}
.editor-frame :deep(code) {
  background: var(--p-surface-100);
  font-size: 0.9em;
}
.editor-frame :deep(blockquote) {
  padding-inline-start: 12px;
  margin: 0.5em 0;
  border-inline-start: 1px solid var(--p-content-border-color);
  color: var(--p-text-muted-color);
}
.editor-frame :deep(a) {
  color: var(--p-primary-700);
  text-decoration: underline;
}
.editor-frame :deep(h1),
.editor-frame :deep(h2),
.editor-frame :deep(h3),
.editor-frame :deep(h4),
.editor-frame :deep(h5),
.editor-frame :deep(h6) {
  font-size: 1em;
  font-weight: 600;
  margin: 0.5em 0;
}
.editor-frame :deep(.comment-mention) {
  color: var(--p-primary-700);
  background: var(--p-primary-50);
  border-radius: 3px;
  padding-inline: 2px;
  font-weight: 600;
  white-space: normal;
}
.editor-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
}
.markdown-help-trigger {
  margin-inline-start: auto;
  min-height: 24px;
  padding-inline: 2px;
  font-size: 12px;
  color: var(--p-text-muted-color);
  text-decoration: underline;
  text-underline-offset: 2px;
  cursor: pointer;
}
.markdown-help {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 6px 12px;
  max-width: min(250px, calc(100vw - 56px));
  margin: 0;
  font-size: 13px;
}
.markdown-help dt {
  font-weight: 600;
}
.markdown-help dd {
  margin: 0;
  overflow-wrap: anywhere;
}
.editor-error {
  margin: 4px 0 0;
  color: var(--p-red-700);
  font-size: 12px;
}
.mention-recovery {
  text-decoration: underline;
  cursor: pointer;
}
.rich-content :deep(.comment-mention-unavailable) {
  text-decoration: underline wavy var(--p-red-700);
  text-underline-offset: 3px;
}
.mention-options {
  z-index: 1100;
  width: min(320px, calc(100vw - 32px));
  box-sizing: border-box;
  list-style: none;
  padding: 4px;
  margin: 0;
  max-height: 208px;
  overflow: auto;
  border: 1px solid var(--p-content-border-color);
  border-radius: var(--p-border-radius-md);
  background: var(--p-content-background);
  box-shadow: 0 4px 12px rgb(0 0 0 / 12%);
}
.mention-options button {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 2px;
  width: 100%;
  text-align: start;
  font: inherit;
  font-size: 13px;
  padding: 8px;
  min-height: 40px;
  border: 0;
  background: transparent;
  color: var(--p-text-color);
  cursor: pointer;
}
.mention-person {
  display: flex;
  align-items: baseline;
  gap: 8px;
  max-width: 100%;
}
.mention-detail {
  font-size: 12px;
}
.mention-options strong,
.mention-email {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mention-email {
  font-size: 12px;
}
.mention-options [aria-selected='true'] button {
  background: var(--p-highlight-background);
  color: var(--p-highlight-color);
}
.mention-options .mention-hovered:not([aria-selected='true']) button {
  background: var(--p-content-hover-background);
}
.mention-state {
  padding: 8px;
  font-size: 12px;
  color: var(--p-text-muted-color);
}
</style>
