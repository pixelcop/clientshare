<script setup lang="ts">
import { storeToRefs } from 'pinia';

import { useAuthStore } from '@/stores/auth';

import FileManager from './FileManager.vue';
import PublicLinkAccess from './PublicLinkAccess.vue';

const { user } = storeToRefs(useAuthStore());
const { folderId } = defineProps<{ folderId?: string | null }>();

const fileView = computed(() => {
  if (user.value?.role === 'link') {
    return PublicLinkAccess;
  } else {
    return FileManager;
  }
});
</script>

<template>
  <component :is="fileView" :folderId="folderId || undefined" />
</template>
