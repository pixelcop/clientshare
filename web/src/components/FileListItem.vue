<script setup lang="ts">
import type { FileItem } from '@/types/models';
import { formatDate, formatDateNatural, formatSize, getFileIcon } from '@/types/util';

import FileRowActions from './FileRowActions.vue';

const { item } = defineProps<{ item: FileItem }>();
const emit = defineEmits<{ open: [item: FileItem] }>();
</script>

<template>
  <tr class="file-row">
    <td class="file-name-cell">
      <button
        type="button"
        class="file-name"
        :title="item.filename"
        @click.stop="emit('open', item)"
      >
        <i
          :class="item.type === 'folder' ? 'oi oi-folder' : getFileIcon(item)"
          aria-hidden="true"
        />
        <span>{{ item.filename }}</span>
      </button>
    </td>
    <td class="file-metadata hidden sm:table-cell">
      {{ item.type === 'folder' ? '—' : formatSize(item.size) }}
    </td>
    <td class="file-metadata hidden sm:table-cell" :title="formatDate(item.uploaded_at)">
      {{ formatDateNatural(item.uploaded_at) }}
    </td>
    <td class="file-actions-cell"><FileRowActions :item @open="emit('open', item)" /></td>
  </tr>
</template>
