<script setup lang="ts">
import { Button, Dialog, Select } from 'openvue';

import AccessSelect from '@/components/AccessSelect.vue';
import { useCollectionStore } from '@/stores/collection';
import type { LinkAccessMode } from '@/types/linkAccess';
import {
  formatLinkExpiry,
  linkAccessDescription,
  linkAccessLabel,
  linkAccessOptions,
} from '@/types/linkAccess';
import type { AccessCapabilities, SecureLink } from '@/types/models';

import SecureLinkResult from './SecureLinkResult.vue';

const workflow = useCollectionStore();
const props = defineProps<{
  show?: boolean;
  embedded?: boolean;
  loading?: boolean;
  error?: string | null;
  defaultExpiryDays?: number;
  defaultAccessType?: LinkAccessMode;
  defaultSendEmail?: boolean;
  collection?: boolean;
  targetName?: string;
  result?: SecureLink | null;
  capabilities?: AccessCapabilities;
}>();
const emit = defineEmits<{
  submit: [
    payload: {
      accessType: LinkAccessMode;
      expiryDays: number;
      expiresAt: string;
      email: string;
      sendEmail: boolean;
    },
  ];
  close: [];
}>();
const purpose = ref<'send' | 'collect'>(props.collection ? 'collect' : 'send');
const accessType = ref<LinkAccessMode>('read');
const allowedAccess = computed(() =>
  linkAccessOptions
    .filter(
      (option) =>
        !props.capabilities ||
        ((option.value !== 'read' || props.capabilities.can_read) &&
          (option.value !== 'write' || props.capabilities.can_upload) &&
          (option.value !== 'readwrite' ||
            (props.capabilities.can_read && props.capabilities.can_upload))),
    )
    .map((option) => ({ ...option, description: linkAccessDescription(option.value) })),
);
const allowsMode = (mode: LinkAccessMode) =>
  allowedAccess.value.some((option) => option.value === mode);
const expiryDays = ref<number | string>(props.defaultExpiryDays || 90);
const anchorTime = ref(Date.now());
const expiryOptions = [7, 14, 30, 60, 90, 120];
const expiryValid = computed(
  () =>
    Number.isInteger(Number(expiryDays.value)) &&
    Number(expiryDays.value) >= 1 &&
    Number(expiryDays.value) <= 120,
);
const fieldPrefix = computed(() => (props.embedded ? 'collect-embedded-link' : 'collect-link'));
const targetName = computed(() => props.targetName?.trim() || 'New client workspace');
function normalizeExpiry(value: number | string) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? Math.min(120, Math.max(1, Math.round(parsed))) : 90;
}
const expiresAt = computed(() =>
  new Date(anchorTime.value + normalizeExpiry(expiryDays.value) * 86400000).toISOString(),
);

function resetSettings() {
  // Permission/purpose choices are intentionally not restored from a different
  // client/item dialog. Collect always starts at upload-only, never readwrite.
  purpose.value = props.collection ? 'collect' : 'send';
  accessType.value = props.collection ? 'write' : props.defaultAccessType || 'read';
  if (!allowsMode(accessType.value)) {
    accessType.value = (allowedAccess.value[0]?.value as LinkAccessMode) || 'read';
    purpose.value = accessType.value === 'write' ? 'collect' : 'send';
  }
  expiryDays.value = normalizeExpiry(props.defaultExpiryDays || 90);
  anchorTime.value = Date.now();
}
function setPurpose(value: 'send' | 'collect') {
  if (!allowsMode(value === 'collect' ? 'write' : 'read')) {
    return;
  }
  purpose.value = value;
  accessType.value = value === 'collect' ? 'write' : 'read';
}
function getSettings() {
  return {
    valid: expiryValid.value && allowsMode(accessType.value),
    accessType: accessType.value,
    expiryDays: normalizeExpiry(expiryDays.value),
    expiresAt: expiresAt.value,
    sendEmail: false,
    email: '',
  };
}
defineExpose({ getSettings, resetSettings });
watch(
  () => props.show,
  (show) => {
    if (show) {
      resetSettings();
    }
  },
  { immediate: true },
);
const dialogComponent = computed(() => (props.embedded ? 'div' : Dialog));
const formComponent = computed(() => (props.embedded ? 'div' : 'form'));
</script>

<template>
  <component
    :is="dialogComponent"
    :visible="!!show"
    @update:visible="(visible: boolean) => !visible && emit('close')"
    @show="collection && workflow.guidance && !embedded && !result && workflow.guide('link-access')"
    modal
    blockScroll
    :header="`Share client workspace: ${targetName}`"
    :aria-label="`Share client workspace: ${targetName}`"
    :closable="!loading"
    :closeOnEscape="!loading"
    :draggable="false"
    :style="{ width: embedded ? undefined : '34rem', maxWidth: 'calc(100vw - 2rem)' }"
  >
    <SecureLinkResult v-if="result" :key="result.id" :link="result" :targetName />
    <component
      v-else
      :is="formComponent"
      :id="`${fieldPrefix}-form`"
      class="link-settings"
      @submit.prevent="expiryValid && emit('submit', getSettings())"
    >
      <p class="link-scope">Sharing: entire client workspace · {{ targetName }}</p>
      <fieldset class="link-purpose" :disabled="loading">
        <legend>What would you like to do?</legend>
        <label
          ><input
            type="radio"
            :name="`${fieldPrefix}-purpose`"
            :checked="purpose === 'send'"
            :disabled="!allowsMode('read')"
            @change="setPurpose('send')"
          />
          Send documents</label
        >
        <label
          ><input
            type="radio"
            :name="`${fieldPrefix}-purpose`"
            :checked="purpose === 'collect'"
            :disabled="!allowsMode('write')"
            @change="setPurpose('collect')"
          />
          Collect uploads</label
        >
      </fieldset>
      <div>
        <label :for="`${fieldPrefix}-access`">Permissions</label>
        <AccessSelect
          :inputId="`${fieldPrefix}-access`"
          v-model="accessType"
          :options="allowedAccess"
          optionLabel="label"
          optionValue="value"
          fluid
          :disabled="loading"
        />
        <p class="link-note">{{ linkAccessDescription(accessType) }}</p>
      </div>
      <div>
        <label :for="`${fieldPrefix}-expiry`">Expires in (days)</label>
        <Select
          :inputId="`${fieldPrefix}-expiry`"
          v-model="expiryDays"
          :options="expiryOptions"
          :invalid="!expiryValid"
          editable
          fluid
          :disabled="loading"
        />
        <p class="link-note">1–120 days. Expires {{ formatLinkExpiry(expiresAt) }}.</p>
        <p v-if="!expiryValid" class="link-error" role="alert">
          Enter a whole number from 1 to 120 days.
        </p>
      </div>
      <section class="link-confirmation" aria-label="Access summary">
        <p>
          <strong>Anyone with this link</strong> can use
          <strong>{{ linkAccessLabel(accessType) }}</strong> in <strong>{{ targetName }}</strong
          >.
        </p>
        <p>Includes all files and folders in this client.</p>
        <p>Expires {{ formatLinkExpiry(expiresAt) }}.</p>
      </section>
      <p class="link-note">After creating the link, you can copy it or send it by email.</p>
      <p v-if="error" role="alert" class="link-error">{{ error }}</p>
    </component>
    <template v-if="!embedded" #footer>
      <div class="link-footer">
        <Button v-if="result" label="Done" severity="secondary" @click="emit('close')" />
        <template v-else>
          <Button
            type="button"
            label="Cancel"
            severity="secondary"
            :disabled="loading"
            @click="emit('close')"
          />
          <Button
            type="submit"
            :form="`${fieldPrefix}-form`"
            label="Create link"
            :loading="loading"
            :disabled="!expiryValid"
          />
        </template>
      </div>
    </template>
  </component>
</template>

<style scoped>
.link-settings {
  display: grid;
  gap: 16px;
  font-size: 14px;
}
.link-settings p {
  margin: 0;
}
.link-scope {
  font-weight: 600;
}
.link-note {
  margin-top: 8px;
  color: var(--p-text-muted-color);
  font-size: 12px;
  line-height: 1.5;
}
.link-settings label,
.link-purpose legend {
  display: block;
  font-weight: 500;
  margin-bottom: 8px;
}
.link-purpose {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 16px;
}
.link-purpose label {
  display: inline-flex;
  align-items: center;
  min-height: 44px;
  gap: 8px;
  margin: 0;
}
.link-confirmation {
  display: grid;
  gap: 8px;
  background: var(--p-content-hover-background);
  border: 1px solid var(--p-content-border-color);
  border-radius: 8px;
  padding: 16px;
}
.link-footer {
  display: flex;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 8px;
}
.link-footer :deep(button) {
  min-height: 44px;
}
.link-error {
  color: var(--p-red-600);
}
</style>
