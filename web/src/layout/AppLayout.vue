<script setup lang="ts">
import { storeToRefs } from 'pinia';

import CollectDocument from '@/components/CollectDocument.vue';
import HelpLauncher from '@/components/HelpLauncher.vue';
import PasskeyEnrollmentDialog from '@/components/PasskeyEnrollmentDialog.vue';
import { useLayout } from '@/composables/useLayout';
import { useAuthStore } from '@/stores/auth';

import AppTopbar from './AppTopbar.vue';

const { user } = storeToRefs(useAuthStore());
const { isPublicLayout } = useLayout();
const route = useRoute();
const isRequestFile = computed(() => route.name === 'DocumentRequestFile');
const isFilePage = computed(() => route.name === 'FileView' || isRequestFile.value);
const isRequestRecipient = computed(() => route.name === 'DocumentRequestRecipient');
const isAnonymousRequest = computed(
  () => (isRequestRecipient.value || isRequestFile.value) && !user.value,
);
</script>

<template>
  <AppTopbar v-if="(!!user || isRequestRecipient) && !isFilePage" />

  <router-view v-if="isFilePage" :key="isRequestFile ? route.fullPath : undefined" />
  <div v-else class="layout-main-container">
    <div
      v-if="isPublicLayout || isAnonymousRequest"
      class="layout-main flex-1 w-full max-w-6xl mx-auto p-4 md:p-8"
    >
      <router-view :key="isRequestRecipient ? route.fullPath : undefined" />
    </div>
    <div v-else class="layout-main">
      <CollectDocument v-if="user?.role === 'admin' && !isRequestRecipient" />
      <router-view :key="isRequestRecipient ? route.fullPath : undefined" />
    </div>
  </div>

  <PasskeyEnrollmentDialog v-if="!isAnonymousRequest" />
  <HelpLauncher :key="user?.id || 'login'" />
  <ConfirmDialog dismissableMask />
  <Toast />
</template>
