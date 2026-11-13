<script setup lang="ts">
import Menu from 'openvue/menu';
import type { MenuItem } from 'openvue/menuitem';
import { computed, ref } from 'vue';

const props = defineProps<{
  guideURL: string;
  supportLabel?: string;
  supportURL?: string;
  contactAvailable: boolean;
  loading: boolean;
  gettingStarted?: boolean;
  showLauncher?: boolean;
  floating?: boolean;
}>();
const visible = defineModel<boolean>('visible', { required: true });
const emit = defineEmits<{ open: []; start: []; contact: [] }>();
const menu = ref<InstanceType<typeof Menu>>();
const items = computed<MenuItem[]>(() => {
  const result: MenuItem[] = [
    {
      label: 'User guide',
      description: 'Find answers and step-by-step instructions',
      icon: 'oi oi-book',
      url: props.guideURL,
      target: '_blank',
    },
  ];
  if (props.gettingStarted) {
    result.push({
      label: 'Getting started: Collect a document',
      description: 'Walk through your first document request',
      icon: 'oi oi-send',
      command: () => emit('start'),
    });
  }
  if (props.contactAvailable || props.supportURL) {
    result.push(
      { separator: true },
      {
        label: props.supportLabel,
        description: props.contactAvailable
          ? 'Send a question or share feedback'
          : 'Get help from your portal operator',
        icon: 'oi oi-comments',
        ...(props.contactAvailable
          ? { command: () => emit('contact') }
          : { url: props.supportURL, target: '_blank' }),
      },
    );
  } else if (props.loading) {
    result.push(
      { separator: true },
      { label: 'Loading support options…', disabled: true, icon: 'oi oi-spin oi-spinner' },
    );
  }
  return result;
});
</script>

<template>
  <Teleport v-if="showLauncher" to="#help-launcher">
    <Button
      id="help-menu-trigger"
      label="Help"
      icon="oi oi-question-circle"
      severity="secondary"
      outlined
      aria-haspopup="menu"
      aria-controls="help-menu"
      @click="menu?.toggle($event)"
    />
    <Menu
      ref="menu"
      id="help-menu"
      :model="items"
      popup
      aria-label="Help and resources"
      class="help-menu"
      @show="emit('open')"
    >
      <template #start>
        <div class="px-4 pt-3 pb-2 font-semibold text-color">Help &amp; resources</div>
      </template>
      <template #item="{ item, props: itemProps }">
        <a
          v-bind="itemProps.action"
          :href="item.url"
          :target="item.target"
          :rel="item.target ? 'noopener noreferrer' : undefined"
          class="help-menu-item"
        >
          <i :class="item.icon" class="help-menu-icon" aria-hidden="true" />
          <span class="min-w-0 flex-1">
            <span class="block font-medium">{{ item.label }}</span>
            <span v-if="item.description" class="mt-1 block text-sm text-muted-color">{{
              item.description
            }}</span>
          </span>
          <i
            v-if="item.target"
            class="oi oi-external-link text-xs text-muted-color"
            aria-hidden="true"
          />
        </a>
      </template>
    </Menu>
  </Teleport>
  <Dialog
    v-model:visible="visible"
    modal
    dismissableMask
    :draggable="false"
    :position="floating ? 'bottomright' : 'center'"
    :header="supportLabel"
    :style="{
      width: '32rem',
      maxWidth: 'calc(100vw - 2rem)',
      ...(floating ? { marginBottom: '5rem', maxHeight: 'calc(100dvh - 6rem)' } : {}),
    }"
  >
    <slot />
  </Dialog>
</template>

<style scoped>
.help-menu {
  width: 23rem;
  max-width: calc(100vw - 2rem);
}

.help-menu-item {
  gap: 0.875rem;
  padding: 0.875rem;
  align-items: center;
}

.help-menu-icon {
  color: var(--p-text-muted-color);
  font-size: 1.125rem;
}
</style>
