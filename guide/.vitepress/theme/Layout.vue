<script setup lang="ts">
import DefaultTheme from 'vitepress/theme';
import Logo from '../../../web/src/components/Logo.vue';

const { Layout } = DefaultTheme;
</script>

<template>
  <Layout>
    <template #nav-bar-title-before>
      <Logo class="docs-wordmark" />
    </template>
    <template #home-hero-info-before>
      <Logo class="docs-hero-wordmark" />
    </template>
  </Layout>
</template>
