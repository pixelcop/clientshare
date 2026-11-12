<script setup lang="ts">
import { Button, Checkbox, InputGroup, InputText } from 'openvue';

import AccessSelect from '@/components/AccessSelect.vue';
import type { SharingPanelSession } from '@/composables/useSharingPanel';

import SharingExpiry from './SharingExpiry.vue';

const { session } = defineProps<{ session: SharingPanelSession }>();
const {
  scope,
  key,
  view,
  links,
  busy,
  history,
  activeTab,
  scopeSummary,
  isMobile,
  copied,
  resultHeading,
  active,
  modes,
  hasLinkHistory,
  visibleLinks,
  date,
  linkMode,
  linkExpiry,
  result,
  recipient,
  createLink,
  createAnotherLink,
  useLink,
  copy,
  emailLink,
  confirmRemove,
} = session;

function openedAt(value: string) {
  return new Date(value).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  });
}
</script>

<template>
  <div>
    <Button
      v-if="!isMobile"
      label="Back to sharing"
      icon="oi oi-arrow-left"
      severity="secondary"
      variant="text"
      :disabled="busy"
      @click="activeTab = 'people'"
    />
    <section aria-label="Sharing links">
      <form
        v-if="!result && modes.length"
        class="access-form link-composer"
        @submit.prevent="createLink"
      >
        <h4>Create a link for {{ scope.name }}</h4>
        <div class="link-settings">
          <div class="link-setting">
            <label :id="`${key}-mode-label`" :for="`${key}-mode`">Anyone with this link can</label>
            <AccessSelect
              v-model="linkMode"
              :inputId="`${key}-mode`"
              :ariaLabelledby="`${key}-mode-label`"
              :options="modes"
              optionLabel="label"
              optionValue="value"
              :disabled="busy"
              fluid
              class="link-role-select"
            />
          </div>
          <div class="link-setting">
            <span class="setting-label">Link expiry</span>
            <SharingExpiry v-model="linkExpiry" :disabled="busy" label="Link expiry" required />
          </div>
        </div>
        <p class="muted">
          Anyone with the link can use it until it expires or you revoke it. Removing a person from
          sharing won’t stop them using this link.
        </p>
        <Button
          type="submit"
          label="Create link"
          :loading="busy"
          :disabled="!linkMode || !linkExpiry"
        />
      </form>
      <div
        v-if="result && active(result)"
        :id="`${key}-link-details`"
        class="access-form"
        aria-label="Created link"
      >
        <h4 :ref="(element) => (resultHeading = element as HTMLElement | null)" tabindex="-1">
          Link ready: {{ scope.name }}
        </h4>
        <p class="muted">
          {{ modes.find((mode) => mode.value === result?.access_type)?.label }} ·
          {{ scopeSummary }} · Expires {{ date(result.expires_at) }}
        </p>
        <p class="muted">Copy the link to share it yourself, or send it by email below.</p>
        <label :for="`${key}-url`">Link address</label>
        <InputGroup class="link-address">
          <InputText :id="`${key}-url`" :modelValue="result.link_url" readonly fluid />
          <Button
            :label="copied ? 'Copied' : 'Copy link'"
            :icon="copied ? 'oi oi-check' : 'oi oi-copy'"
            :disabled="!result.link_url"
            @click="copy(result)"
          />
        </InputGroup>
        <span class="copy-status" role="status">{{ copied ? 'Link copied.' : '' }}</span>
        <form class="link-email-form" @submit.prevent="emailLink">
          <label :for="`${key}-link-email`">Email link to</label>
          <InputGroup>
            <InputText :id="`${key}-link-email`" v-model="recipient" type="email" required fluid />
            <Button type="submit" label="Email link" severity="secondary" :loading="busy" />
          </InputGroup>
        </form>
        <Button
          label="Create another link"
          severity="secondary"
          variant="text"
          class="create-another-link"
          :disabled="busy"
          @click="createAnotherLink"
        />
      </div>
      <section v-if="links.length" class="existing-links" aria-label="Existing links">
        <div class="links-heading">
          <h4>Existing links</h4>
          <label v-if="hasLinkHistory" class="history-toggle" :for="`${key}-history`">
            <Checkbox v-model="history" binary :inputId="`${key}-history`" />
            Show inactive links
          </label>
        </div>
        <p v-if="history && hasLinkHistory" class="muted link-history-note">
          Includes expired and revoked links. These links no longer work.
        </p>
        <p v-if="!visibleLinks.length" class="muted">No active links.</p>
        <div v-for="link in visibleLinks" :key="link.id" class="access-row">
          <div>
            <strong>{{
              link.access_type === 'write'
                ? 'Upload only'
                : link.access_type === 'readwrite'
                  ? 'View, download & upload'
                  : 'View & download'
            }}</strong>
            <small>{{
              link.revoked_at
                ? 'Revoked'
                : active(link)
                  ? `Expires ${date(link.expires_at)}`
                  : `Expired ${date(link.expires_at)}`
            }}</small>
            <small class="link-usage">{{
              link.last_accessed
                ? `Last opened ${openedAt(link.last_accessed)}`
                : 'No recorded opens'
            }}</small>
          </div>
          <div class="row-buttons">
            <Button
              v-if="active(link) && link.link_url"
              :label="result?.id === link.id ? 'Selected' : 'Copy or email'"
              severity="secondary"
              :aria-controls="result?.id === link.id ? `${key}-link-details` : undefined"
              :disabled="busy || result?.id === link.id"
              @click="useLink(link)"
            />
            <Button
              v-if="active(link)"
              label="Revoke"
              severity="secondary"
              :disabled="busy"
              @click="confirmRemove({ kind: 'link', item: link })"
            />
          </div>
        </div>
        <p class="muted link-history-note">No recorded opens doesn’t guarantee a link is unused.</p>
      </section>
      <router-link
        v-if="scope.kind === 'client' && view?.can_manage"
        :to="`/clients/${scope.id}/links`"
        class="management-link"
        >Manage links</router-link
      >
    </section>
  </div>
</template>

<style scoped src="@/assets/sharing-panel.css"></style>

<style scoped>
.link-address {
  min-width: 0;
}
.link-address :deep(input) {
  min-width: 0;
  text-overflow: ellipsis;
}
.link-address :deep(.p-button) {
  flex-shrink: 0;
}
.copy-status {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip-path: inset(50%);
}
.access-form {
  display: grid;
  gap: 12px;
  margin-top: 20px;
}
.access-form > p {
  margin: 0;
}
.link-email-form {
  display: grid;
  gap: 8px;
  margin-top: 8px;
}
.link-email-form :deep(input) {
  min-width: 0;
}
.link-email-form :deep(.p-button) {
  flex-shrink: 0;
}
.create-another-link {
  justify-self: start;
}
.link-settings {
  display: grid;
  grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr);
  gap: 12px;
}
.link-setting {
  display: grid;
  align-content: start;
  gap: 8px;
  min-width: 0;
}
.link-setting > label,
.setting-label {
  font-size: 12px;
  font-weight: 500;
  color: var(--p-text-muted-color);
}
.link-setting :deep(.p-select) {
  min-width: 0;
  min-height: 44px;
}
.link-setting :deep(.link-role-select .p-select-label) {
  display: flex;
  align-items: center;
  min-height: 42px;
  padding-block: 0;
}
.link-composer > .muted,
.link-history-note {
  font-size: 12px;
}
.link-composer > .p-button {
  justify-self: end;
}
.existing-links {
  margin-top: 24px;
  padding-top: 16px;
  border-top: 1px solid var(--p-content-border-color);
}
.links-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 8px 16px;
}
.links-heading h4,
.links-heading .history-toggle {
  margin: 0;
}
.links-heading .history-toggle {
  font-size: 12px;
  color: var(--p-text-muted-color);
  cursor: pointer;
}
.management-link {
  display: inline-flex;
  align-items: center;
  min-height: 44px;
  margin-top: 12px;
  color: var(--p-primary-color);
  text-decoration: underline;
}
@media (max-width: 480px) {
  .link-settings {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
