<script setup lang="ts">
import { storeToRefs } from 'pinia';
import Avatar from 'primevue/avatar';
import Menu from 'primevue/menu';

import { useLayout } from '@/composables/useLayout';
import { useBrandingStore } from '@/stores/branding';

import { useAuthStore } from '../stores/auth';

const { isDarkTheme, toggleDarkMode } = useLayout();
const router = useRouter();
const auth = useAuthStore();
const { user } = storeToRefs(auth);
const { branding } = storeToRefs(useBrandingStore());

const menuRef = ref();

const userInitials = computed(() => {
  if (!user.value?.name) {
    return '';
  }
  return user.value.name
    .split(' ')
    .map((n: string) => n[0])
    .join('')
    .toUpperCase();
});

function toggleMenu(event: MouseEvent) {
  menuRef.value.toggle(event);
}

async function logout() {
  await auth.logout();
  void router.push('/');
}

const menuItems = computed(() => {
  const items = [];

  if (isDarkTheme.value) {
    items.push({
      label: 'Switch to Light Mode',
      icon: 'pi pi-sun',
      command: toggleDarkMode,
    });
  } else {
    items.push({
      label: 'Switch to Dark Mode',
      icon: 'pi pi-moon',
      command: toggleDarkMode,
    });
  }

  if (user.value?.role !== 'link') {
    items.push({
      label: 'Security',
      icon: 'pi pi-shield',
      command: () => router.push({ name: 'Security' }),
    });
  }

  if (user.value?.is_initial_admin && branding.value.accountDashboardURL) {
    items.push({
      label: 'Account & billing',
      icon: 'pi pi-external-link',
      url: branding.value.accountDashboardURL,
    });
  }

  items.push({ label: 'Logout', icon: 'pi pi-sign-out', command: logout });

  return items;
});
</script>

<template>
  <template v-if="user">
    <Avatar
      v-if="userInitials"
      shape="circle"
      size="large"
      :label="userInitials"
      class="cursor-pointer"
      @click="toggleMenu"
    />
    <Avatar
      v-else
      shape="circle"
      size="large"
      icon="pi pi-user"
      class="cursor-pointer"
      @click="toggleMenu"
    />
    <Menu ref="menuRef" :model="menuItems" popup class="mt-4" />
  </template>
</template>

<style scoped>
.layout-topbar-action :deep(span) {
  display: block;
  font-size: 1.25rem;
}
</style>
