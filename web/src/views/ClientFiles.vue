<script setup lang="ts">
import { storeToRefs } from 'pinia';

import { useAuthStore } from '@/stores/auth';

import FileManager from './FileManager.vue';
import PublicClientAccess from './PublicClientAccess.vue';
import PublicLinkAccess from './PublicLinkAccess.vue';

const { user } = storeToRefs(useAuthStore());

const fileView = computed(() => {
  if (user.value?.role === 'link') {
    return PublicLinkAccess;
  } else if (user.value?.role === 'client' || user.value?.role === 'customer') {
    return PublicClientAccess;
  } else {
    return FileManager;
  }
});
</script>

<template>
  <component :is="fileView" :v-bind="$props" />
</template>
