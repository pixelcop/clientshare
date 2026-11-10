<script setup lang="ts">
import { Button, DatePicker, Dialog, Select } from 'openvue';

const value = defineModel<Date | null>({ required: true });
const {
  disabled = false,
  required = false,
  compact = false,
  label = 'Expiry',
} = defineProps<{
  disabled?: boolean;
  required?: boolean;
  compact?: boolean;
  label?: string;
}>();
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
const zone = new Intl.DateTimeFormat(undefined, { timeZoneName: 'short' })
  .formatToParts(new Date())
  .find((part) => part.type === 'timeZoneName')?.value;
watch(
  value,
  (next) => {
    const preset =
      next &&
      ['7', '30'].find(
        (days) => Math.abs(next.getTime() - Date.now() - Number(days) * 86400000) < 60000,
      );
    choice.value = !next ? 'none' : preset || 'custom';
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
  custom.value = false;
}
</script>

<template>
  <Select
    :modelValue="choice"
    :options="options"
    optionLabel="label"
    optionValue="value"
    :ariaLabel="`${label}: ${summary}`"
    :title="summary"
    :disabled="disabled"
    class="sharing-expiry"
    :class="{ 'compact-expiry': compact }"
    :pt="{ dropdown: compact ? { style: { display: 'none' } } : undefined }"
    :labelStyle="{
      display: 'flex',
      alignItems: 'center',
      gap: '8px',
      justifyContent: compact ? 'center' : undefined,
    }"
    @update:modelValue="select"
  >
    <template #value>
      <i class="oi oi-clock" aria-hidden="true" />
      <span v-if="!compact">{{ summary }}</span>
    </template>
  </Select>
  <Dialog
    v-model:visible="custom"
    header="Custom expiry"
    modal
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
}
.compact-expiry:hover,
.compact-expiry:focus-within {
  border-color: var(--p-form-field-border-color);
  background: var(--p-content-hover-background);
}
.timezone-hint {
  color: var(--p-text-muted-color);
  font-size: 12px;
  margin-top: 12px;
}
</style>
