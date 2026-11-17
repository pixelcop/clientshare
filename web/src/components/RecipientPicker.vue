<script setup lang="ts">
import { AutoComplete, Button } from 'openvue';

import { useRecipientSearch } from '@/composables/useRecipientSearch';
import { useAuthStore } from '@/stores/auth';
import type { RecipientContact } from '@/types/recipients';
import { recipientContext } from '@/types/recipients';

defineOptions({ inheritAttrs: false });
const props = withDefaults(
  defineProps<{
    inputId: string;
    clientId?: string;
    label?: string;
    placeholder?: string;
    disabled?: boolean;
    autofocus?: boolean;
    required?: boolean;
  }>(),
  { clientId: '', label: 'Recipient', placeholder: 'Name or email address' },
);
const email = defineModel<string>({ required: true });
// Keep dialog autofocus quiet; typing and subsequent focus still offer contacts.
const initialAutofocusPending = ref(props.autofocus);
const auth = useAuthStore();
const enabled = computed(() => ['admin', 'manager'].includes(auth.user?.role || ''));
const { contacts, across, loading, error, available, search, invalidate } = useRecipientSearch(
  () => ({
    clientId: props.clientId,
    enabled: enabled.value,
    actorKey: `${auth.user?.tenant_id || ''}:${auth.user?.id || ''}`,
  }),
);
const selected = ref<RecipientContact | null>(null);
watch(email, (value) => {
  if (selected.value?.email !== value) {
    selected.value = null;
  }
});
watch(
  () => props.clientId,
  () => {
    selected.value = null;
  },
);
function update(value: string | RecipientContact | null) {
  invalidate();
  selected.value = value && typeof value === 'object' ? value : null;
  email.value = typeof value === 'string' ? value : value?.email || '';
}
function toggleScope() {
  across.value = !across.value;
  document.getElementById(props.inputId)?.focus();
}
</script>

<template>
  <div class="recipient-picker" data-sentry-mask>
    <div v-if="enabled && available" class="recipient-scope">
      <span>{{ across ? 'Across authorized clients' : 'Current client contacts' }}</span>
      <Button
        type="button"
        :label="across ? 'Use current client' : 'Search across clients'"
        variant="link"
        size="small"
        :disabled="disabled || !clientId"
        :aria-pressed="across"
        @click="toggleScope"
      />
    </div>
    <div class="recipient-input">
      <AutoComplete
        :modelValue="email"
        :inputId="inputId"
        :suggestions="contacts"
        appendTo="self"
        optionLabel="email"
        dataKey="id"
        :ariaLabel="label"
        :placeholder="placeholder"
        :disabled="disabled"
        :loading="loading"
        :minLength="0"
        :delay="250"
        :completeOnFocus="enabled && !initialAutofocusPending"
        :autoOptionFocus="false"
        :forceSelection="false"
        :showEmptyMessage="enabled && available"
        :emptySearchMessage="
          !enabled
            ? 'Enter an email address.'
            : across
              ? 'No matching contacts. Enter a new email address.'
              : 'No matching client contacts. Enter an email or search across clients.'
        "
        :pt="{
          pcInputText: { root: { type: 'email', required, autofocus, autocomplete: 'off' } },
          overlay: { 'data-sentry-mask': true },
        }"
        fluid
        @update:modelValue="update"
        @complete="search($event.query)"
        @focus="initialAutofocusPending = false"
      >
        <template #option="{ option }">
          <div class="recipient-option">
            <strong>{{ option.name || option.email }}</strong>
            <span v-if="option.name">{{ option.email }}</span>
            <small>
              {{ recipientContext(option, clientId) }}
              <template v-if="clientId && option.clients.length">
                · {{ option.clients.map((client: { name: string }) => client.name).join(' · ') }}
              </template>
            </small>
          </div>
        </template>
      </AutoComplete>
      <slot name="action" />
    </div>
    <small v-if="selected" class="recipient-context" role="status">
      {{ recipientContext(selected, clientId) }}. Selecting a contact does not change access.
    </small>
    <small v-if="error" class="recipient-context" role="status">{{ error }}</small>
  </div>
</template>

<style scoped>
.recipient-picker {
  flex: 1;
  min-width: 0;
}
.recipient-input {
  display: flex;
  align-items: flex-start;
  gap: 0.5rem;
}
.recipient-input :deep(.p-autocomplete) {
  flex: 1;
  min-width: 0;
}
.recipient-option {
  display: grid;
  gap: 0.25rem;
  min-width: 0;
  white-space: normal;
  overflow-wrap: anywhere;
}
.recipient-option strong {
  font-weight: 600;
}
.recipient-option span,
.recipient-option small,
.recipient-scope,
.recipient-context {
  color: var(--p-text-muted-color);
  font-size: 0.8125rem;
}
.recipient-scope {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.25rem 0.5rem;
  margin-bottom: 0.25rem;
}
.recipient-scope :deep(.p-button) {
  padding: 0.5rem 0;
}
.recipient-context {
  display: block;
  margin-top: 0.25rem;
}
</style>
