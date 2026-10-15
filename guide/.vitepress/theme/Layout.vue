<script setup lang="ts">
import DefaultTheme from 'vitepress/theme';
import { withBase } from 'vitepress';
import { onMounted, shallowRef } from 'vue';
import Logo from '../../../web/src/components/Logo.vue';

const { Layout } = DefaultTheme;
const siteLink = shallowRef<{ url: string; label: string } | null>(null);

onMounted(async () => {
  try {
    const response = await fetch(withBase('/site-config.json'), {
      credentials: 'omit',
      cache: 'no-store',
    });
    if (!response.ok) return;
    const config = await response.json();
    const link = config?.site_link;
    if (typeof link?.url !== 'string' || typeof link?.label !== 'string') return;
    const url = link.url.trim();
    const label = link.label.trim();
    if (!url || !label || /[\\\r\n\t]/.test(url) || url.startsWith('//')) return;
    const relative = url.startsWith('/');
    const parsed = new URL(url, window.location.origin);
    if (!relative && !/^https?:\/\//i.test(url)) return;
    if (!['http:', 'https:'].includes(parsed.protocol) || parsed.username || parsed.password) return;
    siteLink.value = { url, label };
  } catch {
    // The optional site link stays hidden when runtime config is unavailable.
  }
});
</script>

<template>
  <Layout>
    <template #nav-bar-title-before>
      <Logo class="docs-wordmark" />
    </template>
    <template #home-hero-info-before>
      <Logo class="docs-hero-wordmark" />
    </template>
    <template #nav-bar-content-after>
      <a
        v-if="siteLink"
        class="docs-site-link docs-site-link-desktop"
        :href="siteLink.url"
        :title="siteLink.label"
        target="_self"
      >{{ siteLink.label }}</a>
    </template>
    <template #nav-screen-content-before>
      <a
        v-if="siteLink"
        class="docs-site-link docs-site-link-mobile"
        :href="siteLink.url"
        target="_self"
      >{{ siteLink.label }}</a>
    </template>
  </Layout>
</template>

<style scoped>
.docs-site-link {
  color: var(--vp-c-text-1);
  font-size: 14px;
  font-weight: 500;
}

.docs-site-link:hover {
  color: var(--vp-c-brand-1);
}

.docs-site-link-desktop {
  display: none;
  margin-left: 16px;
  max-width: 14rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.docs-site-link-mobile {
  display: block;
  padding: 12px 0;
  border-bottom: 1px solid var(--vp-c-divider);
  overflow-wrap: anywhere;
}

@media (min-width: 960px) {
  .docs-site-link-desktop {
    display: block;
  }
}
</style>
