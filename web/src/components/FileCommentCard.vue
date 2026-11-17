<script setup lang="ts">
import { Button, Menu } from 'openvue';
import type { MenuItem } from 'openvue/menuitem';
import { computed, ref, useId, watch } from 'vue';

import type { FileComment } from '@/types/models';

const props = defineProps<{
  comment: FileComment;
  disabled?: boolean;
  editing?: boolean;
  historyOpen?: boolean;
}>();
const emit = defineEmits<{ edit: []; remove: []; history: [] }>();
const menu = ref<InstanceType<typeof Menu> | null>(null);
const menuId = `comment-actions-${useId()}`;
const initials = computed(() =>
  props.comment.author_name
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map((word) => Array.from(word)[0] || '')
    .join('')
    .toUpperCase(),
);
const timestamp = computed(() =>
  new Date(props.comment.created_at).toLocaleString(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }),
);
const actions = computed<MenuItem[]>(() => {
  const items: MenuItem[] = [];
  if (props.comment.can_edit && !props.editing) {
    items.push({ label: 'Edit comment', icon: 'oi oi-pencil', command: () => emit('edit') });
  }
  if (props.comment.can_view_history) {
    items.push({
      label: props.historyOpen ? 'Hide history' : 'View history',
      icon: 'oi oi-history',
      command: () => emit('history'),
    });
  }
  if (props.comment.can_edit && !props.editing) {
    items.push(
      { separator: true },
      {
        label: 'Remove comment',
        icon: 'oi oi-trash',
        command: () => emit('remove'),
      },
    );
  }
  return items;
});
watch(
  () => props.disabled,
  (disabled) => {
    if (disabled) {
      menu.value?.hide();
    }
  },
);
</script>

<template>
  <article class="comment-card" :aria-label="`Comment by ${comment.author_name}`">
    <header class="comment-card-header">
      <span class="comment-avatar" aria-hidden="true">{{ initials }}</span>
      <div class="comment-identity">
        <div class="comment-byline">
          <strong>{{ comment.author_name }}</strong>
          <span class="comment-audience" :class="{ 'team-note': comment.visibility === 'team' }">
            <i v-if="comment.visibility === 'team'" class="oi oi-lock" aria-hidden="true" />
            {{ comment.visibility === 'team' ? 'Team only' : 'Shared' }}
          </span>
        </div>
        <div class="comment-time">
          <time :datetime="comment.created_at">{{ timestamp }}</time>
          <span v-if="comment.version > 1 && !comment.deleted_at"> · Edited</span>
        </div>
      </div>
      <Button
        v-if="actions.length"
        icon="oi oi-ellipsis-h"
        aria-label="Comment actions"
        title="Comment actions"
        aria-haspopup="menu"
        :aria-controls="menuId"
        severity="secondary"
        variant="text"
        class="comment-menu-trigger"
        :disabled="disabled"
        @click="menu?.toggle($event)"
      />
      <Menu
        v-if="actions.length"
        :id="menuId"
        ref="menu"
        :model="actions"
        popup
        aria-label="Comment actions"
        :pt="{ itemLink: { class: 'min-h-11' } }"
        data-sentry-mask
      />
    </header>
    <div class="comment-card-content"><slot /></div>
  </article>
</template>

<style scoped>
.comment-card {
  border: 1px solid var(--p-content-border-color);
  border-radius: 8px;
  background: var(--p-content-background);
}
.comment-card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background: var(--p-surface-50);
  border-bottom: 1px solid var(--p-content-border-color);
  border-radius: 7px 7px 0 0;
}
.comment-avatar {
  display: grid;
  place-items: center;
  flex: 0 0 28px;
  height: 28px;
  border: 1px solid var(--p-content-border-color);
  border-radius: 50%;
  background: var(--p-content-background);
  color: var(--p-text-muted-color);
  font-size: 11px;
  font-weight: 600;
}
.comment-identity {
  flex: 1;
  min-width: 0;
}
.comment-byline {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px 8px;
  overflow-wrap: anywhere;
}
.comment-byline strong {
  font-size: 13px;
  font-weight: 600;
  line-height: 1.5;
}
.comment-audience {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 1px solid var(--p-content-border-color);
  border-radius: 4px;
  padding: 0 4px;
  color: var(--p-text-muted-color);
  font-size: 10px;
  line-height: 1.6;
  white-space: nowrap;
}
.team-note {
  color: var(--p-text-color);
}
.comment-audience i {
  font-size: 9px;
}
.comment-time {
  margin-top: 2px;
  font-size: 11px;
  line-height: 1.5;
  color: var(--p-text-muted-color);
}
.comment-menu-trigger {
  flex: 0 0 40px;
  width: 40px;
  height: 40px;
}
.comment-card-content {
  padding: 12px;
}
</style>
