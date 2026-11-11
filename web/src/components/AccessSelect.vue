<script setup lang="ts">
import Select from 'openvue/select';

import { roleSummary, type TenantAccessRole } from '@/types/accessRoles';

defineOptions({ inheritAttrs: false });
const props = defineProps<{ isFile?: boolean }>();
const attrs = useAttrs();

function label(option: Record<string, unknown>) {
  const key = attrs.optionLabel;
  return typeof key === 'function' ? key(option) : option[String(key || 'label')];
}
function description(option: Record<string, unknown>) {
  return 'can_read' in option
    ? roleSummary(option as unknown as TenantAccessRole, props.isFile)
    : option.description;
}
</script>

<template>
  <Select v-bind="attrs" overlayClass="access-select-overlay" scrollHeight="24rem" checkmark>
    <template #option="{ option }">
      <div class="access-select-option">
        <strong>{{ label(option) }}</strong>
        <small v-if="description(option)">{{ description(option) }}</small>
      </div>
    </template>
  </Select>
</template>

<style>
/* The overlay is teleported to body; keep these styles global and namespaced. */
.access-select-overlay {
  width: min(26rem, calc(100vw - 2rem));
  max-width: calc(100vw - 2rem);
  min-width: min(26rem, calc(100vw - 2rem)) !important;
}
.access-select-overlay .p-select-option {
  min-height: 44px;
  align-items: center;
  white-space: normal;
}
.access-select-option {
  min-width: 0;
  white-space: normal;
  overflow-wrap: anywhere;
}
.access-select-option strong {
  display: block;
  font-weight: 600;
}
.access-select-option small {
  display: block;
  margin-top: 4px;
  color: var(--p-text-muted-color);
  font-size: 12px;
  line-height: 1.5;
}
</style>
