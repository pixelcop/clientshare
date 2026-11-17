<script setup lang="ts">
defineProps<{ body: string; html?: string | null }>();
</script>

<template>
  <!-- Only HTML produced by the backend's safe Markdown renderer is accepted. -->
  <div v-if="html != null" class="comment-markdown" v-html="html" />
  <div v-else class="comment-markdown comment-plain">{{ body }}</div>
</template>

<style scoped>
.comment-markdown {
  overflow-wrap: anywhere;
  min-width: 0;
  font-size: 14px;
  line-height: 1.6;
}
.comment-plain {
  white-space: pre-wrap;
}
.comment-markdown :deep(p) {
  margin: 0 0 0.65em;
}
.comment-markdown :deep(:last-child) {
  margin-bottom: 0;
}
.comment-markdown :deep(ul) {
  list-style: disc;
  padding-left: 1.5em;
  margin-block: 0.65em;
}
.comment-markdown :deep(ol) {
  list-style: decimal;
  padding-left: 1.5em;
  margin-block: 0.65em;
}
.comment-markdown :deep(a) {
  color: var(--p-primary-700);
  text-decoration: underline;
}
.comment-markdown :deep(pre) {
  overflow-x: auto;
  padding: 8px;
  background: var(--p-surface-100);
}
.comment-markdown :deep(code) {
  font-size: 0.9em;
  background: var(--p-surface-100);
}
.comment-markdown :deep(blockquote) {
  margin: 0.65em 0;
  padding-inline-start: 12px;
  border-inline-start: 1px solid var(--p-content-border-color);
  color: var(--p-text-muted-color);
}
.comment-markdown :deep(h1),
.comment-markdown :deep(h2),
.comment-markdown :deep(h3),
.comment-markdown :deep(h4),
.comment-markdown :deep(h5),
.comment-markdown :deep(h6) {
  font-size: 1em;
  font-weight: 600;
  margin: 1em 0 0.4em;
}
.comment-markdown :deep(.comment-mention) {
  color: var(--p-primary-700);
  background: var(--p-primary-50);
  border-radius: 3px;
  padding-inline: 2px;
  font-weight: 600;
}
</style>
