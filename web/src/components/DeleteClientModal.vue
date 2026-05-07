<script setup lang="ts">
defineProps<{ show: boolean; clientName: string; error?: string }>();
defineEmits(['delete', 'close']);
</script>

<template>
  <Dialog :visible="show" modal dismissable-mask closable @update:visible="$emit('close')">
    <template #header>
      <h2 class="mb-0!">Delete Client</h2>
    </template>
    <p>
      Are you sure you want to delete <span class="font-semibold">{{ clientName }}</span
      >?
    </p>
    <div class="flex justify-end gap-2 mt-4">
      <button type="button" @click="$emit('close')" class="px-4 py-2 rounded bg-gray-100">
        Cancel
      </button>
      <button
        type="button"
        @click="$emit('delete')"
        class="px-4 py-2 rounded bg-red-600 text-white font-semibold"
      >
        Delete
      </button>
    </div>
    <div v-if="error" class="mt-2 text-red-600">{{ error }}</div>
  </Dialog>
</template>
