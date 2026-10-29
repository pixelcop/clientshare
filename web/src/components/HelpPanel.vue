<script setup lang="ts">
defineProps<{
  guideURL: string;
  supportLabel?: string;
  supportURL?: string;
  contactAvailable: boolean;
  loading: boolean;
  gettingStarted?: boolean;
}>();
const visible = defineModel<boolean>('visible', { required: true });
const contact = defineModel<boolean>('contact', { required: true });
defineEmits<{ open: []; start: [] }>();
</script>

<template>
  <Button
    v-if="!contactAvailable && !supportURL && !gettingStarted"
    as="a"
    label="User guide"
    :href="guideURL"
    target="_blank"
    rel="noopener noreferrer"
    severity="secondary"
    outlined
  />
  <Button v-else label="Help" severity="secondary" outlined @click="$emit('open')" />
  <Dialog
    v-model:visible="visible"
    modal
    :header="contact ? supportLabel : 'Help'"
    :style="{ width: '32rem', maxWidth: 'calc(100vw - 2rem)' }"
  >
    <div v-if="!contact" class="flex flex-col gap-4">
      <Button
        v-if="gettingStarted"
        label="Getting started: Collect a document"
        @click="
          $emit('start');
          visible = false;
        "
      />
      <a :href="guideURL" target="_blank" rel="noopener noreferrer" class="underline">User guide</a>
      <span v-if="loading" role="status">Loading help options…</span>
      <Button v-else-if="contactAvailable" :label="supportLabel" @click="contact = true" />
      <a
        v-else-if="supportURL"
        :href="supportURL"
        target="_blank"
        rel="noopener noreferrer"
        class="underline"
        >{{ supportLabel }}</a
      >
    </div>
    <slot v-else />
  </Dialog>
</template>
