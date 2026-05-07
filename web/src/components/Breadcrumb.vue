<script setup lang="ts">
import { storeToRefs } from 'pinia';
import { Breadcrumb, type Tag } from 'primevue';
import type { MenuItem } from 'primevue/menuitem';

import { useFileManagerStore } from '@/stores/fileManager';
import type { FileItem } from '@/types/models';

const { file } = defineProps<{
  file?: FileItem;
}>();

const fileManager = useFileManagerStore();
const { breadcrumbs } = storeToRefs(fileManager);

const crumbs = computed<MenuItem[]>(() => {
  const c = breadcrumbs.value.map((item, i) => {
    const t: MenuItem = {
      label: item.filename,
      to: `/folders/${item.id}`,
    };
    if (i === 0) {
      t.icon = 'pi pi-home';
      delete t.label;
    }
    if (!file && i === breadcrumbs.value.length - 1) {
      // last breadcrumb and no file given
      delete t.to;
      t.last = true;
    }
    return t;
  });

  if (file) {
    c.push({
      label: file.filename,
      last: true,
    });
  }

  return c;
});
</script>

<template>
  <Breadcrumb :model="crumbs" class="mb-6 p-0">
    <template #item="{ item }">
      <router-link
        v-if="item.to"
        class="cursor-pointer"
        :class="{ active: item.to === $route.path }"
        :to="item.to"
      >
        <span :class="item.icon"></span>
        <span class="font-semibold">{{ item.label }}</span>
      </router-link>
      <div v-else-if="item.last">
        <Tag :icon="item.icon" :value="item.label" />
      </div>
      <div v-else>
        <span :class="item.icon"></span>
        <span class="font-semibold">{{ item.label }}</span>
      </div>
    </template>
  </Breadcrumb>
</template>

<style scoped>
.active {
  text-decoration: underline;
  text-underline-offset: 4px;
}
</style>
