<script setup lang="ts">
import type { FileItem } from '@/types/models';

import FolderListItem from './FolderListItem.vue';

const { items } = defineProps<{
  items: FileItem[];
}>();

const emit = defineEmits<{
  (e: 'open', item: FileItem): void;
}>();
</script>

<template>
  <div class="space-y-2">
    <div class="flex items-center justify-between">
      <h3 class="text-sm font-semibold text-slate-700">Folders</h3>
      <span v-if="items.length === 0" class="text-xs text-slate-400">No folders</span>
    </div>
    <table v-if="items.length > 0" class="min-w-full">
      <thead>
        <tr>
          <th class="px-0 pr-4 py-2 sm:px-4 sm:py-2 text-left w-full">Name</th>
          <th class="px-4 py-2 text-left hidden sm:table-cell whitespace-nowrap w-[1%]">Added</th>
          <th class="px-0 sm:px-4 py-2 w-[1%]"></th>
        </tr>
      </thead>
      <tbody>
        <FolderListItem v-for="item in items" :key="item.id" :item @open="emit('open', item)" />
      </tbody>
    </table>
  </div>
</template>
