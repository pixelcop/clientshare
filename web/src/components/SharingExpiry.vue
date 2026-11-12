<script setup lang="ts">
import { Button, DatePicker, Dialog, Select } from 'openvue';
import vTooltip from 'openvue/tooltip';

const value = defineModel<Date | null>({ required: true });
const {
  disabled = false,
  required = false,
  compact = false,
  label = 'Expiry',
  preset,
} = defineProps<{
  disabled?: boolean;
  required?: boolean;
  compact?: boolean;
  label?: string;
  preset?: string;
}>();
const emit = defineEmits<{ 'update:preset': [value: string] }>();
const custom = ref(false);
const draft = ref<Date | null>(null);
const choice = ref(value.value ? '7' : 'none');
const options = computed(() => [
  ...(!required ? [{ label: 'No expiry', value: 'none' }] : []),
  { label: 'Expires in 7 days', value: '7' },
  { label: 'Expires in 30 days', value: '30' },
  { label: 'Custom date…', value: 'custom' },
]);
const summary = computed(() => {
  if (!value.value) {
    return 'No expiry';
  }
  if (choice.value === '7' || choice.value === '30') {
    return `Expires in ${choice.value} days`;
  }
  return `Expires ${value.value.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })}`;
});
const expiryDetail = computed(() => {
  if (!value.value) {
    return 'No expiry is set.';
  }
  const days = Math.ceil((value.value.getTime() - Date.now()) / 86400000);
  const relative = days > 0 ? `Expires in ${days} ${days === 1 ? 'day' : 'days'} on` : 'Expired on';
  return `${relative} ${value.value.toLocaleString(undefined, {
    month: 'long',
    day: 'numeric',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    timeZoneName: 'short',
  })}.`;
});
const zone = new Intl.DateTimeFormat(undefined, { timeZoneName: 'short' })
  .formatToParts(new Date())
  .find((part) => part.type === 'timeZoneName')?.value;
watch(
  [value, () => preset],
  ([next, selectedPreset]) => {
    // Composer presets are user choices, not distances to infer again after a remount.
    if (selectedPreset) {
      choice.value = next ? selectedPreset : 'none';
      return;
    }
    const inferredPreset =
      next &&
      ['7', '30'].find(
        (days) => Math.abs(next.getTime() - Date.now() - Number(days) * 86400000) < 60000,
      );
    choice.value = !next ? 'none' : inferredPreset || 'custom';
  },
  { immediate: true },
);
function select(next: string) {
  if (next === 'custom') {
    draft.value = value.value ? new Date(value.value) : null;
    custom.value = true;
    return;
  }
  choice.value = next;
  emit('update:preset', next);
  value.value = next === 'none' ? null : new Date(Date.now() + Number(next) * 86400000);
}
function apply() {
  if (!draft.value) {
    return;
  }
  const end = new Date(draft.value);
  end.setHours(23, 59, 59, 999);
  value.value = end;
  choice.value = 'custom';
  emit('update:preset', 'custom');
  custom.value = false;
}
</script>

<template>
  <Select
    v-tooltip.top="{ value: expiryDetail, disabled: disabled }"
    :modelValue="choice"
    :options="options"
    optionLabel="label"
    optionValue="value"
    :ariaLabel="`${label}: ${summary}`"
    :disabled="disabled"
    class="sharing-expiry"
    :class="{ 'compact-expiry': compact }"
    :pt="{
      label: { 'aria-description': expiryDetail },
      dropdown: compact ? { style: { display: 'none' } } : undefined,
    }"
    :labelStyle="{
      display: 'flex',
      alignItems: 'center',
      gap: '8px',
      justifyContent: compact ? 'center' : undefined,
    }"
    @update:modelValue="select"
  >
    <template #value>
      <span class="expiry-clock" :class="{ 'is-active': !!value }" aria-hidden="true"
        ><i class="oi oi-clock"
      /></span>
      <span v-if="!compact">{{ summary }}</span>
    </template>
    <template #header>
      <p class="expiry-menu-summary">{{ expiryDetail }}</p>
    </template>
  </Select>
  <Dialog
    v-model:visible="custom"
    header="Custom expiry"
    modal
    dismissableMask
    :draggable="false"
    :style="{ width: '24rem', maxWidth: '95vw' }"
  >
    <DatePicker v-model="draft" inline :minDate="new Date()" ariaLabel="Expiry date" />
    <p class="timezone-hint">Access ends at the end of this day ({{ zone }}).</p>
    <template #footer>
      <Button label="Cancel" severity="secondary" variant="text" @click="custom = false" />
      <Button label="Set expiry" :disabled="!draft" @click="apply" />
    </template>
  </Dialog>
</template>

<style scoped>
.sharing-expiry {
  min-height: 44px;
}
.compact-expiry {
  width: 48px;
  flex: 0 0 48px;
  border-color: transparent;
  background: transparent;
  box-shadow: none;
}
.compact-expiry:hover,
.compact-expiry:focus-within {
  border-color: transparent;
  box-shadow: none;
  outline: none;
  background: var(--p-content-hover-background);
}
.compact-expiry:has(:focus-visible) {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 2px;
}
.expiry-clock {
  display: inline-grid;
  place-items: center;
  width: 28px;
  height: 28px;
  flex: 0 0 28px;
  border-radius: 50%;
  font-size: 18px;
  color: var(--p-text-muted-color);
}
.expiry-clock.is-active {
  background: var(--p-highlight-background);
  color: var(--p-highlight-color);
}
.expiry-menu-summary {
  max-width: min(20rem, 80vw);
  padding: 12px;
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--p-text-muted-color);
}
.timezone-hint {
  color: var(--p-text-muted-color);
  font-size: 12px;
  margin-top: 12px;
}
</style>
