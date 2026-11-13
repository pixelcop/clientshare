<script setup lang="ts">
import '@/assets/sharing.css';

import { Button, Checkbox, Dialog } from 'openvue';

import { useSharingPanel } from '@/composables/useSharingPanel';
import type { SharingScope } from '@/types/sharing';

import SharingLinks from './SharingLinks.vue';
import SharingPeople from './SharingPeople.vue';

const { scope } = defineProps<{ scope: SharingScope }>();
const emit = defineEmits<{ close: [] }>();
const session = useSharingPanel(
  () => scope,
  () => emit('close'),
);
const {
  key,
  view,
  loading,
  busy,
  error,
  status,
  activeTab,
  composing,
  isMobile,
  mobileScreen,
  mobileTitle,
  mobileBack,
  pendingChanges,
  discardAction,
  requestClose,
  discardChanges,
  canCreateLinks,
  load,
  saveChanges,
  removal,
  revokeLinks,
  remove,
} = session;
defineExpose({ requestClose });
</script>

<template>
  <div class="sharing-panel" :class="{ 'is-composing': composing }">
    <header v-if="isMobile" class="mobile-sharing-header">
      <Button
        :icon="
          activeTab === 'links' || mobileScreen === 'manage' ? 'oi oi-arrow-left' : 'oi oi-times'
        "
        :aria-label="
          activeTab === 'links' || mobileScreen === 'manage' ? 'Back to share' : 'Close sharing'
        "
        severity="secondary"
        variant="text"
        autofocus
        :disabled="busy"
        @click="activeTab === 'links' || mobileScreen === 'manage' ? mobileBack() : requestClose()"
      />
      <div>
        <h2 aria-live="polite">{{ mobileTitle }}</h2>
        <small data-sentry-mask>{{ scope.name }}</small>
      </div>
    </header>
    <p v-if="scope.kind === 'client' && !isMobile" class="scope-label muted">
      Entire client workspace · all files and folders
    </p>
    <p v-if="loading" role="status">Loading access…</p>
    <div v-if="error" role="alert">
      <p>{{ error }}</p>
      <Button label="Refresh access" severity="secondary" :disabled="busy" @click="load" />
    </div>
    <p v-if="status" data-sentry-mask role="status" class="delivery-status">{{ status }}</p>
    <template v-if="view && !loading">
      <p
        v-if="view.inherits_folder_access && (!isMobile || mobileScreen === 'manage')"
        class="inheritance-note"
      >
        Access also comes from a containing folder. Change the assignment at that folder to remove
        its inherited access.
      </p>
      <SharingPeople v-if="activeTab === 'people'" :session="session" />
      <SharingLinks v-else :session="session" />
      <div v-if="!composing" class="sharing-footer">
        <Button
          v-if="activeTab === 'people' && canCreateLinks && (!isMobile || mobileScreen === 'share')"
          label="Sharing links"
          icon="oi oi-link"
          severity="secondary"
          variant="text"
          @click="activeTab = 'links'"
        />
        <Button
          v-else-if="activeTab === 'links'"
          label="Back to sharing"
          icon="oi oi-arrow-left"
          severity="secondary"
          variant="text"
          :disabled="busy"
          @click="mobileBack"
        />
        <span v-else />
        <div class="footer-actions">
          <span v-if="pendingChanges" class="muted pending-label" role="status"
            >Pending changes</span
          >
          <Button
            :label="pendingChanges ? 'Save' : 'Done'"
            :loading="busy"
            :disabled="busy"
            @click="pendingChanges ? saveChanges() : requestClose()"
          />
        </div>
      </div>
    </template>
    <Dialog
      :visible="!!removal"
      header="Remove access?"
      modal
      :dismissableMask="!busy"
      :draggable="false"
      :style="{ width: '30rem', maxWidth: '95vw' }"
      :closable="!busy"
      :closeOnEscape="!busy"
      @update:visible="(value) => !value && (removal = null)"
    >
      <p v-if="removal?.kind === 'member'">
        Remove their access to this client and its files and folders, and cancel their pending
        invitations. Shares they created for other people stay active. They may still have staff
        access or use a link.
      </p>
      <p v-else-if="removal?.kind === 'link'">
        This link will stop working, even for people who already have it open. Other links and
        people’s account access are unchanged.
      </p>
      <p v-else>
        This removes the invitation or access added here. The person may still have access through
        the client, another folder or a link.
      </p>
      <p v-if="removal?.kind === 'member' || removal?.kind === 'grant'" class="muted">
        Removal takes effect when you save.
      </p>
      <label v-if="removal?.kind === 'member'" class="history-toggle">
        <Checkbox v-model="revokeLinks" binary :inputId="`${key}-revoke-links`" /> Also turn off
        every sharing link for this client
      </label>
      <p v-if="error" role="alert">{{ error }}</p>
      <template #footer>
        <Button label="Cancel" severity="secondary" :disabled="busy" @click="removal = null" />
        <Button label="Remove access" severity="danger" :loading="busy" @click="remove" />
      </template>
    </Dialog>
    <Dialog
      :visible="!!discardAction"
      header="Discard unsaved changes?"
      modal
      dismissableMask
      :draggable="false"
      :style="{ width: '28rem', maxWidth: '95vw' }"
      @update:visible="(value) => !value && (discardAction = null)"
    >
      <p>Your unsaved sharing changes will be lost.</p>
      <template #footer>
        <Button
          label="Cancel"
          severity="secondary"
          variant="text"
          autofocus
          @click="discardAction = null"
        />
        <Button label="Discard" severity="danger" @click="discardChanges" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped src="@/assets/sharing-panel.css"></style>

<style scoped>
.sharing-panel {
  display: grid;
  gap: 8px;
  color: var(--p-text-color);
  font-size: 14px;
}
.sharing-panel :deep(.p-button) {
  min-height: 44px;
}
.mobile-sharing-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 24px;
}
.mobile-sharing-header h2 {
  margin: 0;
  font-size: 22px;
  font-weight: 600;
}
.mobile-sharing-header small {
  display: block;
  color: var(--p-text-muted-color);
  overflow-wrap: anywhere;
}
.mobile-sharing-header > div {
  min-width: 0;
}
.scope-label {
  font-size: 12px;
  font-weight: 400;
  overflow-wrap: anywhere;
}
.footer-actions {
  display: flex;
  align-items: center;
  gap: 16px;
}
.pending-label {
  font-style: italic;
}
.delivery-status {
  padding: 12px;
  border-inline-start: 3px solid var(--p-primary-color);
}
.sharing-footer {
  position: sticky;
  bottom: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 16px;
  padding-top: 8px;
  background: var(--p-content-background);
}
.inheritance-note {
  margin-top: 16px;
  font-size: 12px;
}
</style>
