<script setup lang="ts">
import axios from 'axios';
import { Button } from 'openvue';

import type { FileItem, PaginatedResult } from '@/types/models';

const items = ref<FileItem[]>([]);
const page = ref(1);
const pages = ref(0);
const loading = ref(false);
const error = ref('');
let version = 0;
async function load(next = 1) {
  const current = ++version;
  loading.value = true;
  error.value = '';
  items.value = [];
  try {
    const result = await axios.get<PaginatedResult<FileItem>>('/api/shared-with-me', {
      params: { page: next, page_size: 20 },
    });
    if (current !== version) {
      return;
    }
    items.value = result.data.items;
    page.value = next;
    pages.value = result.data.pagination.total_pages;
  } catch {
    if (current === version) {
      error.value = 'Shared content could not be loaded. Please try again.';
    }
  } finally {
    if (current === version) {
      loading.value = false;
    }
  }
}
onMounted(() => {
  void load();
});
onBeforeUnmount(() => {
  version += 1;
});
function rights(item: FileItem) {
  return [
    item.capabilities?.can_read && 'View & download',
    item.capabilities?.can_upload && 'Upload',
    item.capabilities?.can_share && 'Share with others',
  ]
    .filter(Boolean)
    .join(' · ');
}
</script>

<template>
  <section class="shared-content">
    <header>
      <h1>Shared with me</h1>
      <p>Files and folders people have shared with you.</p>
    </header>
    <p v-if="loading" role="status">Loading shared content…</p>
    <div v-else-if="error" role="alert">
      <p>{{ error }}</p>
      <Button label="Try again" severity="secondary" @click="load(page)" />
    </div>
    <ul v-else-if="items.length">
      <li v-for="item in items" :key="item.id">
        <router-link :to="item.type === 'folder' ? `/folder/${item.id}` : `/files/${item.id}`">
          <i :class="item.type === 'folder' ? 'oi oi-folder' : 'oi oi-file'" aria-hidden="true" />
          <span
            ><strong data-sentry-mask>{{ item.filename }}</strong
            ><small>{{ rights(item) }}</small></span
          >
          <i class="oi oi-angle-right" aria-hidden="true" />
        </router-link>
      </li>
    </ul>
    <div v-else class="shared-empty">
      <h2>No shared files or folders yet</h2>
      <p>Accept an email invitation to find shared files and folders here.</p>
    </div>
    <nav v-if="pages > 1" aria-label="Shared content pages" class="flex items-center gap-3 mt-4">
      <Button
        label="Previous"
        severity="secondary"
        :disabled="loading || page <= 1"
        @click="load(page - 1)"
      />
      <span>Page {{ page }} of {{ pages }}</span>
      <Button
        label="Next"
        severity="secondary"
        :disabled="loading || page >= pages"
        @click="load(page + 1)"
      />
    </nav>
  </section>
</template>

<style scoped>
.shared-content {
  color: var(--p-text-color);
}
header {
  margin-bottom: 24px;
}
h1 {
  font-size: 28px;
  font-weight: 600;
  margin-bottom: 8px;
}
p,
small {
  color: var(--p-text-muted-color);
  line-height: 1.5;
}
li {
  border-bottom: 1px solid var(--p-content-border-color);
}
li a {
  display: flex;
  gap: 16px;
  align-items: center;
  padding: 16px 8px;
  min-height: 72px;
}
li a:focus-visible {
  outline: 2px solid var(--p-primary-color);
  outline-offset: -2px;
}
li a:hover {
  background: var(--p-content-hover-background);
}
li span {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}
strong,
small {
  display: block;
}
strong {
  font-size: 14px;
  font-weight: 600;
}
small {
  margin-top: 4px;
  font-size: 12px;
}
.shared-empty {
  max-width: 36rem;
  padding: 40px 0;
}
h2 {
  font-size: 18px;
  font-weight: 600;
  margin-bottom: 8px;
}
</style>
