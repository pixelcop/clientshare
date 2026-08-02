<script setup lang="ts">
import { storeToRefs } from 'pinia';

import { useLayout } from '@/composables/useLayout';
import PasskeyEnrollmentDialog from '@/components/PasskeyEnrollmentDialog.vue';
import { useAuthStore } from '@/stores/auth';

import AppFooter from './AppFooter.vue';
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
      <router-view />
    </div>
  </div>

  <AppFooter />
  <PasskeyEnrollmentDialog />
  <ConfirmDialog />
  <Toast />
</template>
