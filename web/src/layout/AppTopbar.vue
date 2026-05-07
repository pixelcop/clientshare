<script setup lang="ts">
import type { MenuItem } from '@embedpdf/vue-pdf-viewer';
import { storeToRefs } from 'pinia';

import Logo from '@/components/Logo.vue';
import UserMenu from '@/components/UserMenu.vue';
import { useAuthStore } from '@/stores/auth';
import { defaultBrandingTitle, useBrandingStore } from '@/stores/branding';

const { branding } = storeToRefs(useBrandingStore());
const { user } = storeToRefs(useAuthStore());
const route = useRoute();

const menuItems = computed<MenuItem[]>(() => {
  if (!user.value) {
    return [];
  }
  const role = user.value.role;
  const items = [];
  if (role === 'admin' || role === 'manager') {
    items.push({
      type: 'custom',
      label: 'Clients',
      to: '/clients',
      props: { active: route.path.startsWith('/clients') },
    });
    items.push({
      type: 'custom',
      label: 'Activity',
      to: '/activity',
      props: { active: route.path.startsWith('/activity') },
    });
    items.push({
      type: 'custom',
      label: 'Users',
      to: '/users',
      props: { active: route.path.startsWith('/users') },
    });
    if (role === 'admin') {
      items.push({
        type: 'custom',
        label: 'Settings',
        to: '/settings',
        props: { active: route.path.startsWith('/settings') },
      });
    }
  } else if (role === 'customer' || role === 'client') {
    items.push({
      type: 'custom',
      label: 'My Files',
      to: '/files',
      props: { active: route.path.startsWith('/files') },
    });
    items.push({
      type: 'custom',
      label: 'Activity',
      to: '/activity',
      props: { active: route.path.startsWith('/activity') },
    });
  }
  return items;
});
</script>

<template>
  <Menubar :model="menuItems" class="layout-topbar border-0">
    <template #start>
      <div class="layout-topbar-logo-container">
        <router-link to="/" class="layout-topbar-logo">
          <img
            v-if="branding.logo"
            :src="branding.logo"
            alt="Logo"
            class="h-8 w-8 object-contain"
          />
          <span v-if="branding.title !== defaultBrandingTitle">{{ branding.title }}</span>
          <Logo v-else />
        </router-link>
      </div>
    </template>

    <template #item="{ item, props }">
      <router-link
        :to="item.to"
        v-bind="props.action"
        class="p-menuitem-link"
        :class="{ 'p-menuitem-link-active': item.props.active }"
      >
        <span v-if="item.icon" :class="['p-menuitem-icon', item.icon]"></span>
        <span class="p-menuitem-text">{{ item.label }}</span>
      </router-link>
    </template>

    <template #end>
      <div class="layout-topbar-actions">
        <div class="layout-config-menu">
          <UserMenu v-if="user" />
        </div>
      </div>
    </template>
  </Menubar>
</template>

<style scoped>
.p-menuitem-link-active {
  border: 1px solid var(--p-menubar-border-color) !important;
  border-radius: var(--p-menubar-border-radius);
  /* background-color: var(--primary-color) !important; */
  color: var(--primary-color-text) !important;
}

.p-menubar.p-menubar-mobile :deep(.p-menubar-button) {
  order: 3;
  margin-left: none;
}
</style>
