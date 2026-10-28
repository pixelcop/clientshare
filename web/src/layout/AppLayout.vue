<script setup lang="ts">
import { storeToRefs } from 'pinia';

import CollectDocument from '@/components/CollectDocument.vue';
import PasskeyEnrollmentDialog from '@/components/PasskeyEnrollmentDialog.vue';
import { useLayout } from '@/composables/useLayout';
import { useAuthStore } from '@/stores/auth';

import AppTopbar from './AppTopbar.vue';

const { user } = storeToRefs(useAuthStore());
const { isPublicLayout } = useLayout();
</script>

<template>
  <AppTopbar v-if="!!user" />

  <div class="layout-main-container">
    <div v-if="isPublicLayout" class="layout-main flex-1 w-full max-w-6xl mx-auto p-4 md:p-8">
      <router-view />
    </div>
    <div v-else class="layout-main">
      <CollectDocument v-if="user?.role === 'admin'" />
      <router-view />
    </div>
  </div>

  <PasskeyEnrollmentDialog />
  <ConfirmDialog />
  <Toast />
</template>
