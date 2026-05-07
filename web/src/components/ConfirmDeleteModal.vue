<script lang="ts" setup>
import { Button, Dialog } from 'primevue';

import { useFileManagerStore } from '@/stores/fileManager';

const { showDelete, item } = defineProps<{
  showDelete: boolean;
  item: any;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
}>();

const fileManager = useFileManagerStore();

function remove() {
  fileManager.deleteItem(item).then(() => {
    emit('close');
  });
}
</script>

<template>
  <!-- Delete Confirmation Modal -->
  <Dialog
    :visible="showDelete"
    @update:visible="
      (val) => {
        if (!val) {
          $emit('close');
        }
      }
    "
    modal
    dismissableMask
    close-on-escape
    header="Delete Item"
    class="max-w-md"
  >
    <p>
      Are you sure you want to delete
      <span class="font-semibold">{{ item?.filename }}</span
      >?
    </p>
    <div class="flex justify-end gap-2 mt-4">
      <Button type="button" @click="$emit('close')" severity="secondary"> Cancel </Button>
      <Button type="button" @click="remove" severity="danger"> Delete </Button>
    </div>
    <div v-if="fileManager.error" class="mt-2 text-red-600">{{ fileManager.error }}</div>
  </Dialog>
</template>
