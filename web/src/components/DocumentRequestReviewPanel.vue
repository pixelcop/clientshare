<script setup lang="ts">
import { computed } from 'vue';

import {
  type DocumentRequest,
  documentRequestLabels,
  requestFileRoute,
} from '@/types/documentRequests';

const props = defineProps<{ request: DocumentRequest; clientId: string; fileId: string }>();
const destinations = computed(() =>
  props.request.items.flatMap((item) =>
    item.files.flatMap((file) =>
      file.id && file.id !== props.fileId && file.round === item.round
        ? [{ id: file.id, name: file.name, itemName: item.name, status: item.status }]
        : [],
    ),
  ),
);
</script>

<template>
  <nav aria-label="Switch request documents" class="request-switcher" data-sentry-mask>
    <div class="switcher-heading">
      <h2>{{ request.title }}</h2>
    </div>
    <ul v-if="destinations.length" class="switcher-list">
      <li v-for="destination in destinations" :key="destination.id">
        <RouterLink
          :to="requestFileRoute(clientId, request.id, destination.id)"
          class="switcher-row"
          replace
        >
          <i class="oi oi-file" aria-hidden="true" />
          <span class="switcher-document">
            <strong>{{ destination.itemName }}</strong>
            <span class="switcher-filename">{{ destination.name }}</span>
            <span class="switcher-status">{{ documentRequestLabels[destination.status] }}</span>
          </span>
          <i class="oi oi-chevron-right" aria-hidden="true" />
        </RouterLink>
      </li>
    </ul>
    <p v-else class="switcher-empty">No other files to open.</p>
    <RouterLink
      :to="{ name: 'DocumentRequests', params: { clientId, requestId: request.id } }"
      class="switcher-back"
    >
      Back to full request
    </RouterLink>
  </nav>
</template>

<style scoped>
.request-switcher {
  font-size: 14px;
  line-height: 1.4;
  overflow-wrap: anywhere;
}
.switcher-heading {
  padding: 14px 14px 6px;
}
.switcher-heading h2 {
  margin: 0;
  font-size: 16px;
  line-height: 1.3;
}
.switcher-list {
  padding: 4px 8px;
}
.switcher-list li + li {
  border-top: 1px solid var(--p-content-border-color);
}
.switcher-row {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: start;
  gap: 8px;
  padding: 10px 6px;
  border-radius: 4px;
}
.switcher-row:hover,
.switcher-back:hover {
  background: var(--p-content-hover-background);
}
.switcher-row > i {
  margin-top: 3px;
  color: var(--p-text-muted-color);
}
.switcher-document {
  display: grid;
  gap: 2px;
}
.switcher-filename {
  font-size: 13px;
}
.switcher-status,
.switcher-empty {
  font-size: 12px;
  color: var(--p-text-muted-color);
}
.switcher-empty {
  margin: 0;
  padding: 12px 14px;
}
.switcher-back {
  display: flex;
  align-items: center;
  min-height: 44px;
  padding: 8px 14px;
  border-top: 1px solid var(--p-content-border-color);
  text-decoration: underline;
  text-underline-offset: 2px;
}
.request-switcher a:focus-visible {
  outline: 2px solid var(--p-primary-color);
  outline-offset: -2px;
}
</style>
