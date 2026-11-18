<script setup lang="ts">
import { shift } from '@floating-ui/dom';
import { PluginKey } from '@tiptap/pm/state';
import { exitSuggestion, type SuggestionProps } from '@tiptap/suggestion';
import { EditorContent, useEditor } from '@tiptap/vue-3';
import axios from 'axios';
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, watch } from 'vue';

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
}>();
const body = defineModel<string>({ required: true });
const mentionKey = new PluginKey(`comment-mentions-${props.id}`);
const suggestion = shallowRef<SuggestionProps<CommentPerson, CommentMentionAttrs> | null>(null);
const selected = ref(0);
const popup = ref<HTMLUListElement | null>(null);
const mentionError = ref('');
const empty = ref(!body.value.trim());
const length = computed(() => Array.from(body.value).length);
let popupGeneration = 0;
let unmountPopup: (() => void) | undefined;
let emittedSource = body.value;

function closePopup() {
  popupGeneration++;
  unmountPopup?.();
  unmountPopup = undefined;
  suggestion.value = null;
  mentionError.value = '';
}
async function openPopup(context: SuggestionProps<CommentPerson, CommentMentionAttrs>) {
  suggestion.value = context;
  selected.value = 0;
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
function choose(person: CommentPerson) {
  suggestion.value?.command({ id: person.id, label: person.name });
}
function editorAttributes() {
  return {
    id: props.id,
    role: 'textbox',
    'aria-label': props.label || 'Comment',
    'aria-multiline': 'true',
    'aria-readonly': String(!!props.readonly),
    'aria-disabled': String(!!props.disabled),
    'aria-invalid': String(length.value > 4000),
    'aria-describedby': `${props.id}-length`,
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
  extensions: commentExtensions({
    pluginKey: mentionKey,
    char: '@',
    allowedPrefixes: [' ', '(', '\n'],
    debounce: 200,
    allow: ({ editor: current }) => !current.isActive('code') && !current.isActive('codeBlock'),
    floatingUi: { strategy: 'fixed', middleware: [shift({ padding: 12 })] },
    items: async ({ query, signal }) => {
      mentionError.value = '';
      if (props.disabled || props.readonly) {
        return [];
      }
      try {
        const { data } = await axios.get<CommentPerson[]>(
          `/api/files/${props.fileId}/comments/mentions`,
          { params: { search: query, visibility: props.visibility }, signal },
        );
        return signal.aborted ? [] : data;
      } catch (cause) {
        if (!signal.aborted && !axios.isCancel(cause)) {
          mentionError.value = 'Mention search unavailable. Try typing the name again.';
        }
        return [];
      }
    },
    render: () => ({
      onStart: (context) => {
        void openPopup(context);
      },
      onUpdate: (context) => {
        if (context.query !== suggestion.value?.query) {
          selected.value = 0;
        }
        suggestion.value = context;
      },
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
          selected.value =
            (selected.value + (event.key === 'ArrowDown' ? 1 : -1) + people.length) % people.length;
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
  }),
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
  [suggestion, selected, () => props.label, length],
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
        class="editor-frame"
        :class="{ 'editor-tall': rows === 4, invalid: length > 4000, disabled }"
      >
        <EditorContent :editor="editor" class="rich-content" />
        <span v-if="empty" class="editor-placeholder" aria-hidden="true"
          >Write a comment · type @ to mention</span
        >
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
          @pointermove="selected = index"
        >
          <button type="button" tabindex="-1" @mousedown.prevent @click="choose(person)">
            <strong>@{{ person.name }}</strong>
            <span v-if="person.email" class="mention-email">{{ person.email }}</span>
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
    <div class="editor-meta">
      <slot name="metadata" />
      <small :id="`${id}-length`" class="editor-length">{{ length }}/4000</small>
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
  align-items: flex-start;
  gap: 8px;
  margin-top: 4px;
}
.editor-length {
  display: block;
  margin-inline-start: auto;
  text-align: end;
  font-variant-numeric: tabular-nums;
  color: var(--p-text-muted-color);
  font-size: 12px;
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
  align-items: baseline;
  gap: 8px;
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
.mention-state {
  padding: 8px;
  font-size: 12px;
  color: var(--p-text-muted-color);
}
</style>
