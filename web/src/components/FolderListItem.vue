<script setup lang="ts">
import { breakpointsTailwind, useBreakpoints } from '@vueuse/core';
import { storeToRefs } from 'pinia';
import { SplitButton } from 'primevue';
import type { MenuItem } from 'primevue/menuitem';

import { useAuthStore } from '@/stores/auth';
import { useFileManagerStore } from '@/stores/fileManager';
import type { FileItem } from '@/types/models';
import { formatDate, formatDateNatural, getFileIcon } from '@/types/util';

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

function downloadZip(item: FileItem) {
  const fileStore = fileManager as ReturnType<typeof useFileManagerStore>;
  fileStore.downloadZip(item);
}

function confirmDelete(item: FileItem) {
  deleteItemObj.value = item;
  showDelete.value = true;
}

const folderActions = computed<MenuItem[]>(() => {
  const actions: MenuItem[] = [
    {
      label: 'Open',
      icon: 'pi pi-folder-open',
      command: () => emit('open', item),
    },
  ];
  actions.push({
    label: 'Download Zip',
    icon: 'pi pi-download',
    command: () => downloadZip(item),
  });
  actions.push({
    label: 'Delete',
    icon: 'pi pi-trash',
    command: () => confirmDelete(item),
  });
  return actions;
});

const xs = breakpoints.smaller('sm');

const actionButtonSize = computed(() => {
  return xs.value ? 'small' : 'normal';
});
</script>

<template>
  <tr>
    <td
      class="px-0 pr-4 sm:px-4 py-2 flex gap-2 cursor-pointer w-full min-w-0"
      @click.stop="$emit('open', item)"
    >
      <i class="text-3xl md:text-lg" :class="getFileIcon(item)"></i>
      {{ item.filename }}
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
            :model="folderActions"
            icon="pi pi-folder-open"
            :size="actionButtonSize"
            @click.stop="emit('open', item)"
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
            label="Open"
            icon="pi pi-folder-open"
            size="small"
            severity="info"
            @click.stop="emit('open', item)"
          />
          <Button
            v-tooltip.left="{ value: 'Download Zip', showDelay: 500, hideDelay: 300 }"
            icon="pi pi-download"
            size="small"
            severity="secondary"
            @click.stop="downloadZip(item)"
          />
        </div>
        <span v-else class="text-xs text-slate-400">Uploads only</span>
      </template>
    </td>
  </tr>
</template>
