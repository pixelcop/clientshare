<script setup lang="ts">
const props = defineProps<{ show: boolean; userName: string; error?: string }>();
const emit = defineEmits(['delete', 'close']);
</script>

<template>
  <div
    v-if="show"
    class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50"
  >
    <div class="bg-white p-6 rounded shadow w-full max-w-sm">
      <h2 class="text-lg font-bold mb-4">Delete User</h2>
      <p>
        Are you sure you want to delete <span class="font-semibold">{{ userName }}</span
        >?
      </p>
      <div class="flex justify-end gap-2 mt-4">
        <button type="button" class="px-4 py-2 rounded bg-gray-100" @click="$emit('close')">
          Cancel
        </button>
        <button
          type="button"
          class="px-4 py-2 rounded bg-red-600 text-white font-semibold"
          @click="$emit('delete')"
        >
          Delete
        </button>
      </div>
      <div v-if="error" class="mt-2 text-red-600">{{ error }}</div>
    </div>
  </div>
</template>

<style scoped></style>
