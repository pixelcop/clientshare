<script setup lang="ts">
import { storeToRefs } from 'pinia';

import { useFolderListingRoute } from '@/composables/useFolderListingRoute';
import { useAuthStore } from '@/stores/auth';

import FileManager from './FileManager.vue';
import PublicLinkAccess from './PublicLinkAccess.vue';

const { user } = storeToRefs(useAuthStore());
const { folderId } = defineProps<{ folderId?: string | null }>();
useFolderListingRoute();

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
