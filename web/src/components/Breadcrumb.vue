<script setup lang="ts">
import { useResizeObserver } from '@vueuse/core';
import { Menu } from 'openvue';
import { storeToRefs } from 'pinia';

import { useAuthStore } from '@/stores/auth';
import { useClientsStore } from '@/stores/clients';
import { useFileManagerStore } from '@/stores/fileManager';
import { usePublicLinkStore } from '@/stores/publicLink';
import type { FileItem } from '@/types/models';
import { breadcrumbHiddenCount } from '@/utils/breadcrumb';

const { file, heading = false } = defineProps<{ file?: FileItem; heading?: boolean }>();
const { breadcrumbs, context } = storeToRefs(useFileManagerStore());
const clients = useClientsStore();
const auth = useAuthStore();
const publicLink = usePublicLinkStore();
const router = useRouter();
const clientName = computed(
  () =>
    context.value?.client_name ||
    (context.value ? breadcrumbs.value[0]?.filename || 'Shared files' : '') ||
    (auth.user?.role === 'link' ? publicLink.client?.name : clients.currentClient?.name) ||
    'Client Files',
);
const rootRoute = computed(() => {
  const id = breadcrumbs.value[0]?.id;
  return id ? `/folder/${id}` : undefined;
});
const crumbs = computed(() => {
  const folders = breadcrumbs.value.slice(1).map((item) => ({
    id: item.id,
    label: item.filename,
    to: `/folder/${item.id}`,
  }));
  if (file) {
    folders.push({ id: file.id, label: file.filename, to: '' });
  }
  return folders;
});
const container = ref<HTMLElement>();
const root = ref<HTMLElement>();
const measurements = ref<HTMLElement>();
const overflowMeasure = ref<HTMLElement>();
const menu = ref<InstanceType<typeof Menu>>();
const hiddenCount = ref(0);
const menuId = `breadcrumb-${useId()}`;
const hiddenItems = computed(() =>
  crumbs.value.slice(0, hiddenCount.value).map((item) => ({
    label: item.label,
    command: () => router.push(item.to),
  })),
);

function measure() {
  if (!container.value || !root.value || !measurements.value || !overflowMeasure.value) {
    return;
  }
  const widths = Array.from(measurements.value.children, (el) => el.getBoundingClientRect().width);
  hiddenCount.value = breadcrumbHiddenCount(
    widths,
    container.value.clientWidth - root.value.getBoundingClientRect().width,
    overflowMeasure.value.getBoundingClientRect().width,
  );
}

useResizeObserver([container, root, measurements], measure);
watch(
  [crumbs, clientName],
  async () => {
    menu.value?.hide();
    await nextTick();
    measure();
  },
  { flush: 'post' },
);
onMounted(measure);
</script>

<template>
  <nav ref="container" class="file-breadcrumb" aria-label="Folder path">
    <h1 v-if="heading" :class="crumbs.length ? 'sr-only' : 'root-heading'">
      {{ crumbs.at(-1)?.label || clientName }}
    </h1>
    <ol v-if="crumbs.length || !heading" class="breadcrumb-list">
      <li ref="root" class="breadcrumb-root" :class="{ 'root-only': !crumbs.length }">
        <router-link
          v-if="rootRoute && crumbs.length"
          :to="rootRoute"
          class="client-link breadcrumb-link"
          :title="clientName"
        >
          <span class="client-label">{{ clientName }}</span>
        </router-link>
        <span v-else class="crumb-label" :title="clientName" aria-current="page">{{
          clientName
        }}</span>
      </li>
      <li v-if="hiddenCount" class="breadcrumb-segment breadcrumb-overflow">
        <i class="oi oi-chevron-right separator" aria-hidden="true" />
        <button
          type="button"
          class="overflow-button"
          aria-label="Show parent folders"
          aria-haspopup="menu"
          :aria-controls="menuId"
          @click="menu?.toggle($event)"
        >
          …
        </button>
      </li>
      <li
        v-for="(item, index) in crumbs.slice(hiddenCount)"
        :key="item.id"
        class="breadcrumb-segment"
        :class="{ 'current-segment': index === crumbs.length - hiddenCount - 1 }"
      >
        <i class="oi oi-chevron-right separator" aria-hidden="true" />
        <span
          v-if="index === crumbs.length - hiddenCount - 1"
          class="crumb-label"
          :title="item.label"
          aria-current="page"
          >{{ item.label }}</span
        >
        <router-link v-else :to="item.to" class="crumb-label breadcrumb-link" :title="item.label">{{
          item.label
        }}</router-link>
      </li>
    </ol>
    <Menu :id="menuId" ref="menu" :model="hiddenItems" popup aria-label="Parent folders" />
    <!-- Intrinsic widths stay independent of the collapsed path, including after font loading. -->
    <div class="breadcrumb-measure" aria-hidden="true" inert>
      <div ref="measurements" class="breadcrumb-list">
        <span v-for="item in crumbs" :key="item.id" class="breadcrumb-segment">
          <i class="oi oi-chevron-right separator" /><span class="crumb-label">{{
            item.label
          }}</span>
        </span>
      </div>
      <span ref="overflowMeasure" class="breadcrumb-segment breadcrumb-overflow">
        <i class="oi oi-chevron-right separator" /><span class="overflow-button">…</span>
      </span>
    </div>
  </nav>
</template>

<style scoped>
.file-breadcrumb {
  position: relative;
  min-width: 0;
  width: 100%;
  color: var(--p-text-color);
  font-size: 1.5rem;
  font-weight: 600;
  line-height: 1.5;
}
.breadcrumb-list {
  display: flex;
  align-items: center;
  min-width: 0;
  margin: 0;
  padding: 0;
  list-style: none;
  white-space: nowrap;
}
.breadcrumb-root {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-shrink: 0;
  max-width: 45%;
}
.root-only {
  max-width: 100%;
}
.root-heading {
  margin: 0;
  font-size: 1.75rem;
  font-weight: 600;
  overflow-wrap: anywhere;
}
.overflow-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  flex-shrink: 0;
  border-radius: 9999px;
}
.client-link {
  display: inline-flex;
  align-items: center;
  min-height: 40px;
  min-width: 0;
  padding: 4px 12px;
  text-decoration: underline;
  text-decoration-thickness: 1px;
  text-decoration-color: color-mix(in srgb, var(--p-text-muted-color) 55%, transparent);
  text-underline-offset: 0.22em;
}
.client-link:hover {
  text-decoration-color: currentColor;
}
.client-label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.breadcrumb-segment {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  padding-left: 0.25rem;
  flex-shrink: 0;
}
.separator {
  flex-shrink: 0;
  font-size: 0.75rem;
  color: var(--p-text-muted-color);
}
.current-segment {
  min-width: 0;
  flex-shrink: 1;
}
.crumb-label {
  min-width: 0;
  min-height: 40px;
  padding: 4px 12px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.breadcrumb-link,
.overflow-button {
  border-radius: 9999px;
  transition: background-color 140ms ease-out;
}
.overflow-button {
  cursor: pointer;
  font: inherit;
  background: transparent;
  border: 0;
}
.breadcrumb-link:hover,
.breadcrumb-link:focus-visible,
.overflow-button:hover,
.overflow-button:focus-visible {
  background: color-mix(in srgb, var(--p-text-color) 8%, transparent);
}
.breadcrumb-link:active,
.overflow-button:active {
  background: color-mix(in srgb, var(--p-text-color) 12%, transparent);
}
a:focus-visible,
button:focus-visible {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 2px;
}
.breadcrumb-measure {
  position: absolute;
  visibility: hidden;
  pointer-events: none;
  width: max-content;
  height: 0;
  overflow: hidden;
}
</style>
