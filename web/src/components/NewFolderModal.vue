<script setup lang="ts">
import { Dialog } from 'primevue';

import { useFileManagerStore } from '@/stores/fileManager';

const { show } = defineProps<{
  show: boolean;
}>();

const emit = defineEmits<{
  (e: 'close'): void;
}>();

const router = useRouter();
const fileManager = useFileManagerStore();

const newFolderName = ref('');

function createFolder() {
  fileManager.createFolder(newFolderName.value).then((folder) => {
    newFolderName.value = '';
    if (folder) {
      router.push(`/folder/${folder.id}`);
    }
    emit('close');
  });
}
</script>

<template>
  <Dialog
    :visible="show"
    @update:visible="
      (val) => {
        if (!val) $emit('close');
      }
    "
    modal
    dismissableMask
    close-on-escape
    header="New Folder"
    class="min-w-96"
  >
    <form @submit.prevent="createFolder" class="flex flex-col justify-between">
      <InputText
        v-model="newFolderName"
        type="text"
        placeholder="Folder name"
        autofocus
        required
        class="mb-8"
      />
      <div class="flex justify-end gap-2">
        <Button type="button" @click="$emit('close')" severity="secondary" label="Cancel" />
        <Button type="submit" label="Create" />
      </div>
    </form>
  </Dialog>
</template>
