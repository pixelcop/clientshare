<script lang="ts" setup>
import { breakpointsTailwind, useBreakpoints } from '@vueuse/core';
import { storeToRefs } from 'pinia';
import { SplitButton } from 'primevue';
import type { MenuItem } from 'primevue/menuitem';

import { useAuthStore } from '@/stores/auth';
import { useFileManagerStore } from '@/stores/fileManager';
import type { FileItem } from '@/types/models';
import { formatDate, formatDateNatural, formatSize, getFileIcon } from '@/types/util';

import ConfirmDeleteModal from './ConfirmDeleteModal.vue';

const { item } = defineProps<{
  item: FileItem;
}>();

const emit = defineEmits<{
  (e: 'open', item: FileItem): void;
}>();

const breakpoints = useBreakpoints(breakpointsTailwind);
const fileManager = useFileManagerStore();

const { canRead } = storeToRefs(fileManager);
const { user } = storeToRefs(useAuthStore());

const isAdminOrManager = computed(() => ['admin', 'manager'].includes(user.value?.role ?? ''));

const showDelete = ref(false);
const deleteItemObj = ref<FileItem | null>(null);

const fileActions = computed<MenuItem[]>(() => {
  const actions: MenuItem[] = [
    {
      label: 'Download',
      icon: 'pi pi-download',
      command: () => fileManager.downloadFile(item),
    },
    {
      label: 'Preview',
      icon: 'pi pi-eye',
      command: () => emit('open', item),
    },
  ];
  actions.push({
    label: 'Delete',
    icon: 'pi pi-trash',
    command: () => confirmDelete(item),
  });
  return actions;
});

function confirmDelete(item: FileItem) {
  deleteItemObj.value = item;
  showDelete.value = true;
}

const xs = breakpoints.smaller('sm');

const actionButtonSize = computed(() => {
  return xs.value ? 'small' : 'normal';
});
</script>

<template>
  <tr>
    <td
      class="px-0 pr-4 sm:px-4 py-2 flex gap-2 cursor-pointer w-full min-w-0"
      @click="$emit('open', item)"
    >
      <i class="text-3xl md:text-lg" :class="getFileIcon(item)"></i>
      {{ item.filename }}
    </td>
    <!-- <td class="px-4 py-2 hidden sm:table-cell whitespace-nowrap w-[1%]">
      {{ isFolder(item) ? 'Folder' : 'File' }}
    </td> -->
    <td class="px-4 py-2 hidden sm:table-cell whitespace-nowrap w-[1%]">
      {{ formatSize(item.size) }}
    </td>
    <td
      class="px-4 py-2 hidden sm:table-cell min-w-24 whitespace-nowrap w-[1%]"
      v-tooltip.bottom="{ value: formatDate(item.uploaded_at), showDelay: 500, hideDelay: 300 }"
    >
      {{ formatDateNatural(item.uploaded_at) }}
    </td>
    <td class="px-0 sm:px-4 py-2 w-[1%] whitespace-nowrap">
      <template v-if="isAdminOrManager">
        <div class="flex justify-end">
          <SplitButton
            :model="fileActions"
            icon="pi pi-download"
            :size="actionButtonSize"
            @click.stop="fileManager.downloadFile(item)"
            severity="info"
          />
          <ConfirmDeleteModal
            v-if="showDelete"
            :showDelete="showDelete"
            :item="deleteItemObj"
            @close="showDelete = false"
          />
        </div>
      </template>

      <template v-else>
        <div v-if="canRead" class="flex gap-2">
          <Button
            label="Preview"
            icon="pi pi-eye"
            size="small"
            severity="info"
            @click.stop="emit('open', item)"
          />
          <Button
            v-tooltip.left="{ value: 'Download', showDelay: 500, hideDelay: 300 }"
            icon="pi pi-download"
            size="small"
            severity="secondary"
            @click.stop="fileManager.downloadFile(item)"
          />
        </div>
        <span v-else class="text-xs text-slate-400">
          {{ canRead ? 'No actions available' : 'Uploads only' }}
        </span>
      </template>
    </td>
  </tr>
</template>
