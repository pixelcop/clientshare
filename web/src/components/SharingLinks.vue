<script setup lang="ts">
import {
  Button,
  Checkbox,
  InputGroup,
  InputText,
  Tab,
  TabList,
  TabPanel,
  TabPanels,
  Tabs,
} from 'openvue';

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
  linkTab,
  scopeSummary,
  copied,
  copiedLinkId,
  resultHeading,
  active,
  modes,
  hasLinkHistory,
  visibleLinks,
  linkExpiryText,
  linkMode,
  linkExpiry,
  result,
  recipient,
  createLink,
  createAnotherLink,
  copy,
  emailLink,
  confirmRemove,
} = session;

const emailExpanded = ref(!!recipient.value);
watch(
  () => result.value?.id,
  () => {
    emailExpanded.value = false;
  },
);
async function revealEmail() {
  emailExpanded.value = true;
  await nextTick();
  document.getElementById(`${key.value}-link-email`)?.focus();
}

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
    <Tabs v-model:value="linkTab" lazy class="sharing-link-tabs">
      <TabList :pt="{ tabList: { 'aria-label': 'Link sharing views' } }">
        <Tab value="create" :disabled="busy">Create a link</Tab>
        <Tab value="manage" :disabled="busy">Manage links</Tab>
      </TabList>
      <TabPanels class="link-tab-panels">
        <TabPanel value="create">
          <form
            v-if="!result && modes.length"
            class="access-form link-composer"
            @submit.prevent="createLink"
          >
            <h4>Create a link for {{ scope.name }}</h4>
            <div class="link-settings">
              <div class="link-setting">
                <label :id="`${key}-mode-label`" :for="`${key}-mode`"
                  >Anyone with this link can</label
                >
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
              Anyone with the link can use it until it expires or you revoke it. Removing a person
              from sharing won’t stop them using this link.
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
              {{ scopeSummary }} ·
              <span :title="new Date(result.expires_at).toLocaleString()">{{
                linkExpiryText(result.expires_at)
              }}</span>
            </p>
            <InputGroup class="link-address">
              <InputText
                :id="`${key}-url`"
                :modelValue="result.link_url"
                aria-label="Link address"
                readonly
                fluid
              />
              <Button
                :label="copied ? 'Copied' : 'Copy link'"
                :icon="copied ? 'oi oi-check' : 'oi oi-copy'"
                :disabled="!result.link_url"
                @click="copy(result)"
              />
            </InputGroup>
            <form
              v-if="emailExpanded"
              :id="`${key}-email-form`"
              class="link-email-form"
              @submit.prevent="emailLink"
            >
              <InputGroup>
                <InputText
                  :id="`${key}-link-email`"
                  v-model="recipient"
                  placeholder="Email link to"
                  aria-label="Email link to"
                  autocomplete="email"
                  type="email"
                  :disabled="busy"
                  required
                  fluid
                />
                <Button type="submit" label="Email link" severity="secondary" :loading="busy" />
              </InputGroup>
            </form>
            <div class="link-actions">
              <Button
                v-if="!emailExpanded"
                label="Email link"
                variant="link"
                size="small"
                class="email-link-toggle"
                :aria-expanded="false"
                :aria-controls="`${key}-email-form`"
                :disabled="busy"
                @click="revealEmail"
              />
              <Button
                label="Create another link"
                severity="secondary"
                variant="text"
                :disabled="busy"
                @click="createAnotherLink"
              />
            </div>
          </div>
        </TabPanel>
        <TabPanel value="manage">
          <section class="existing-links" aria-label="Existing links">
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
            <p v-if="!links.length" class="muted">No links created yet.</p>
            <p v-else-if="!visibleLinks.length" class="muted">No active links.</p>
            <div v-for="link in visibleLinks" :key="link.id" class="access-row">
              <div>
                <strong>{{
                  link.access_type === 'write'
                    ? 'Upload only'
                    : link.access_type === 'readwrite'
                      ? 'View, download & upload'
                      : 'View & download'
                }}</strong>
                <small
                  :title="link.expires_at ? new Date(link.expires_at).toLocaleString() : undefined"
                  >{{ link.revoked_at ? 'Revoked' : linkExpiryText(link.expires_at) }}</small
                >
                <small class="link-usage">{{
                  link.last_accessed
                    ? `Last opened ${openedAt(link.last_accessed)}`
                    : 'No recorded opens'
                }}</small>
              </div>
              <div class="row-buttons">
                <Button
                  v-if="active(link) && link.link_url"
                  :label="copiedLinkId === link.id ? 'Copied' : 'Copy'"
                  severity="secondary"
                  :disabled="busy"
                  @click="copy(link)"
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
          </section>
          <router-link
            v-if="scope.kind === 'client' && view?.can_manage"
            :to="`/clients/${scope.id}/links`"
            class="management-link"
            >Manage links</router-link
          >
        </TabPanel>
      </TabPanels>
    </Tabs>
    <span class="copy-status" role="status">{{ copiedLinkId ? 'Link copied.' : '' }}</span>
  </div>
</template>

<style scoped src="@/assets/sharing-panel.css"></style>

<style scoped>
.link-tab-panels {
  padding: 0;
}
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
.link-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px 16px;
}
.email-link-toggle {
  padding-inline: 0;
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
  margin-top: 0;
  padding-top: 16px;
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
