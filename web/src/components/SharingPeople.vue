<script setup lang="ts">
import { Button, Dialog } from 'openvue';

import AccessSelect from '@/components/AccessSelect.vue';
import RecipientPicker from '@/components/RecipientPicker.vue';
import type { SharingPanelSession } from '@/composables/useSharingPanel';
import { GROUPS_UI_ENABLED } from '@/features';
import { replaySensitiveTitlePT } from '@/services/replayPrivacy';
import { useAuthStore } from '@/stores/auth';
import { defaultAccessRole, roleCapabilities } from '@/types/accessRoles';
import { describeCapabilities } from '@/types/sharing';
import { withinCeiling } from '@/types/sharingRoles';

import ScopedGroupAccess from './ScopedGroupAccess.vue';
import SharingExpiry from './SharingExpiry.vue';

const { session } = defineProps<{ session: SharingPanelSession }>();
const auth = useAuthStore();
const {
  scope,
  key,
  view,
  busy,
  email,
  drafts,
  composing,
  isMobile,
  mobileScreen,
  mobilePerson,
  memberEdits,
  grantEdits,
  removeRole,
  removeOption,
  memberRoles,
  grantRoles,
  grantEdit,
  setGrantRole,
  setMemberRole,
  isFile,
  catalog,
  roleLabel,
  canInvite,
  roleOptions,
  selectedRole,
  active,
  pastPeople,
  visibleGrants,
  visibleInvitations,
  previewPeople,
  mobilePersonDetails,
  mobileRoleLabel,
  chooseMobileRole,
  removeMobilePerson,
  setMobileExpiry,
  emailMobileInvitation,
  date,
  addPerson,
  removeDraft,
  updateDraft,
  backToPeople,
  load,
  sendInvitation,
  sendPeople,
  canEdit,
  confirmRemove,
  openMobileManage,
} = session;
</script>

<template>
  <div v-if="view" :class="{ 'mobile-share-content': isMobile && mobileScreen === 'share' }">
    <div v-if="drafts.length" class="draft-people" aria-label="People to invite">
      <div v-for="person in drafts" :key="person.id" class="draft-person">
        <div class="recipient-settings">
          <div data-sentry-mask class="recipient-identity">
            <span class="person-avatar" aria-hidden="true">{{
              person.email[0]?.toUpperCase()
            }}</span>
            <strong>{{ person.email }}</strong>
          </div>
          <div class="recipient-controls">
            <AccessSelect
              v-model="person.role"
              :ariaLabel="`Role for ${person.email}`"
              :options="roleOptions"
              optionLabel="label"
              optionValue="value"
              :disabled="busy"
              class="role-select"
              :labelStyle="{ display: 'flex', alignItems: 'center', textAlign: 'start' }"
              @update:modelValue="updateDraft(person)"
            />
            <SharingExpiry
              v-if="scope.kind === 'item'"
              v-model="person.expiry"
              compact
              :label="`Expiry for ${person.email}`"
              :disabled="busy"
              @update:modelValue="updateDraft(person)"
            />
            <Button
              icon="oi oi-times"
              :aria-label="`Remove ${person.email}`"
              severity="secondary"
              variant="text"
              :disabled="busy"
              @click="removeDraft(person.id)"
            />
          </div>
        </div>
      </div>
    </div>
    <form
      v-if="canInvite && (!isMobile || mobileScreen === 'share')"
      class="add-person"
      @submit.prevent="addPerson"
    >
      <label
        v-if="
          !drafts.length &&
          roleOptions.length &&
          !defaultAccessRole(catalog.filter((r) => roleOptions.some((o) => o.value === r.id)))
        "
        class="grid gap-2 mb-3"
        >Access role<AccessSelect
          v-model="selectedRole"
          :options="roleOptions"
          optionLabel="label"
          optionValue="value"
          ariaLabel="Access role for new invitation"
          placeholder="Choose the permissions to grant"
          :disabled="busy"
      /></label>
      <div class="recipient-entry">
        <RecipientPicker
          :inputId="`${key}-email`"
          v-model="email"
          :clientId="view.client_id || (scope.kind === 'client' ? scope.id : '')"
          :placeholder="drafts.length ? 'Add another name or email' : 'Name or email address'"
          :autofocus="!isMobile"
          required
          :disabled="busy || !roleOptions.length"
        >
          <template #action>
            <Button
              v-if="email.trim()"
              type="submit"
              label="Add"
              :disabled="busy || !selectedRole"
            />
          </template>
        </RecipientPicker>
      </div>
      <p v-if="!roleOptions.length" class="muted">You don’t have a shareable role for this item.</p>
    </form>
    <div v-if="drafts.length" class="person-actions">
      <Button
        label="Cancel"
        severity="secondary"
        variant="text"
        :disabled="busy"
        @click="backToPeople()"
      />
      <Button
        label="Send invitation"
        :loading="busy"
        :disabled="!!email.trim()"
        @click="sendPeople"
      />
    </div>
    <section
      v-if="!composing && (!isMobile || mobileScreen === 'manage')"
      aria-label="People with access"
      class="people-section"
    >
      <ScopedGroupAccess
        v-if="GROUPS_UI_ENABLED"
        :key="key"
        :scope="scope"
        :clientId="view.client_id || (scope.kind === 'client' ? scope.id : '')"
        :roles="
          catalog.filter((r) => withinCeiling(roleCapabilities(r, isFile), view!.capabilities))
        "
        :assignments="(view.additional_access || []).filter((a) => a.group_id)"
        @changed="load"
      />
      <h3>People with access</h3>
      <p
        v-if="!view.people?.length && !visibleGrants.length && !visibleInvitations.length"
        class="muted"
      >
        No people added here yet.
      </p>
      <p v-if="view.people && !view.can_manage" class="muted">
        You can remove people you invited. Other people’s access is managed by the portal team.
      </p>
      <component
        :is="isMobile && person.can_remove ? 'button' : 'div'"
        v-for="person in view.people"
        :key="person.user_id"
        class="access-row person-row"
        :class="{
          'mobile-person-row': isMobile,
          'pending-removal': memberEdits[person.user_id] === removeRole,
        }"
        :type="isMobile && person.can_remove ? 'button' : undefined"
        :disabled="isMobile && person.can_remove ? busy : undefined"
        :aria-label="
          isMobile && person.can_remove ? `Change access for ${person.email}` : undefined
        "
        @click="isMobile && person.can_remove && (mobilePerson = { kind: 'member', item: person })"
      >
        <div data-sentry-mask class="person-identity">
          <span class="person-avatar" aria-hidden="true">{{
            (person.name || person.email)[0]?.toUpperCase()
          }}</span>
          <div>
            <strong
              >{{ person.name || person.email
              }}{{ person.user_id === auth.user?.id ? ' (you)' : '' }}</strong
            >
            <small>{{ person.email }}</small>
            <small v-if="isMobile" class="mobile-person-role">{{
              person.role === 'admin'
                ? 'Administrator'
                : mobileRoleLabel(memberEdits[person.user_id] || person.role_id)
            }}</small>
          </div>
        </div>
        <i v-if="isMobile && person.can_remove" class="oi oi-chevron-down" aria-hidden="true" />
        <div v-if="!isMobile" class="recipient-controls">
          <AccessSelect
            v-if="person.can_remove"
            :modelValue="memberEdits[person.user_id] || person.role_id"
            :options="[...memberRoles(person), removeOption]"
            optionLabel="name"
            optionValue="id"
            :ariaLabel="`Workspace role for ${person.email}`"
            :placeholder="roleLabel(person.role_id)"
            :disabled="busy"
            class="inline-role-select"
            @update:modelValue="setMemberRole(person, $event)"
          />
          <span v-else class="role-label" :title="describeCapabilities(person.capabilities)">{{
            person.role === 'admin' ? 'Administrator' : roleLabel(person.role_id)
          }}</span>
        </div>
      </component>
      <component
        :is="isMobile && canEdit(grant) ? 'button' : 'div'"
        v-for="grant in visibleGrants"
        :key="grant.id"
        class="access-row person-row"
        :class="{
          'mobile-person-row': isMobile,
          'pending-removal': grantEdit(grant).role === removeRole,
        }"
        :type="isMobile && canEdit(grant) ? 'button' : undefined"
        :disabled="isMobile && canEdit(grant) ? busy : undefined"
        :aria-label="
          isMobile && canEdit(grant) ? `Change access for ${grant.recipient_email}` : undefined
        "
        @click="isMobile && canEdit(grant) && (mobilePerson = { kind: 'grant', item: grant })"
      >
        <div data-sentry-mask class="person-identity">
          <span class="person-avatar" aria-hidden="true">{{
            (grant.recipient_name || grant.recipient_email)[0]?.toUpperCase()
          }}</span>
          <div>
            <strong>{{ grant.recipient_name || grant.recipient_email }}</strong>
            <small
              >{{ grant.recipient_email
              }}<template v-if="grant.assignment"> · Additional access</template
              ><template v-if="grantEdit(grant).expiry">
                · Expires {{ date(grantEdit(grant).expiry!.toISOString()) }}</template
              ></small
            >
            <small v-if="isMobile" class="mobile-person-role">{{
              mobileRoleLabel(grantEdit(grant).role)
            }}</small>
          </div>
        </div>
        <i v-if="isMobile && canEdit(grant)" class="oi oi-chevron-down" aria-hidden="true" />
        <div v-if="!isMobile" class="recipient-controls">
          <AccessSelect
            v-if="canEdit(grant)"
            :id="`${key}-grant-${grant.id}`"
            :modelValue="grantEdit(grant).role"
            :options="[...grantRoles(grant), removeOption]"
            optionLabel="name"
            optionValue="id"
            :isFile="isFile"
            :ariaLabel="`Role for ${grant.recipient_email}`"
            :disabled="busy"
            class="inline-role-select"
            @update:modelValue="setGrantRole(grant, $event)"
          />
          <span v-else class="role-label" :title="describeCapabilities(grant)">{{
            roleLabel(grant.role_id)
          }}</span>
          <SharingExpiry
            v-if="canEdit(grant) && scope.kind === 'item'"
            :modelValue="grantEdit(grant).expiry"
            compact
            :label="`Expiry for ${grant.recipient_email}`"
            :disabled="busy || grantEdit(grant).role === removeRole"
            @update:modelValue="grantEdits[grant.id] = { ...grantEdit(grant), expiry: $event }"
          />
        </div>
      </component>
      <component
        :is="isMobile && active(invitation) ? 'button' : 'div'"
        v-for="invitation in visibleInvitations"
        :key="invitation.id"
        class="access-row"
        :class="{ 'mobile-person-row': isMobile }"
        :type="isMobile ? 'button' : undefined"
        :disabled="isMobile ? busy : undefined"
        :aria-label="isMobile ? `Invitation options for ${invitation.email}` : undefined"
        @click="
          isMobile &&
          active(invitation) &&
          (mobilePerson = { kind: 'invitation', item: invitation })
        "
      >
        <span v-if="isMobile" data-sentry-mask class="person-avatar" aria-hidden="true">{{
          invitation.email[0]?.toUpperCase()
        }}</span>
        <div class="mobile-invitation-identity">
          <strong data-sentry-mask>{{ invitation.email }}</strong>
          <small
            >{{
              invitation.accepted_at
                ? 'Accepted'
                : invitation.revoked_at
                  ? 'Removed'
                  : active(invitation)
                    ? 'Awaiting acceptance'
                    : 'Expired'
            }}
            · {{ roleLabel(invitation.role_id)
            }}<template v-if="invitation.grant_expires_at">
              · Access expires {{ date(invitation.grant_expires_at) }}</template
            ></small
          >
        </div>
        <i v-if="isMobile && active(invitation)" class="oi oi-chevron-down" aria-hidden="true" />
        <details v-if="!isMobile && active(invitation)" class="invitation-actions">
          <summary :aria-label="`Invitation options for ${invitation.email}`">Pending</summary>
          <div class="row-buttons">
            <Button
              label="Email invitation"
              severity="secondary"
              variant="text"
              :disabled="busy"
              @click="sendInvitation(invitation)"
            />
            <Button
              label="Revoke"
              severity="secondary"
              variant="text"
              :disabled="busy"
              @click="confirmRemove({ kind: 'invitation', item: invitation })"
            />
          </div>
        </details>
      </component>
      <details v-if="pastPeople.length" class="past-access">
        <summary>Past invitations & access ({{ pastPeople.length }})</summary>
        <div v-for="person in pastPeople" :key="person.id" class="access-row">
          <div>
            <strong data-sentry-mask>{{ person.email }}</strong
            ><small>{{ person.state }}</small>
          </div>
        </div>
      </details>
    </section>
  </div>
  <section v-if="isMobile && mobileScreen === 'share' && !composing" class="mobile-access-preview">
    <button type="button" class="mobile-manage-access" :disabled="busy" @click="openMobileManage">
      <span
        ><strong>People with access</strong
        ><span class="mobile-avatar-stack" aria-hidden="true"
          ><span
            v-for="person in previewPeople.slice(0, 5)"
            data-sentry-mask
            :key="person.email"
            class="person-avatar"
            >{{ person.name[0]?.toUpperCase() }}</span
          ><small v-if="!previewPeople.length">No people added yet</small></span
        ></span
      >
      <span>Manage access <i class="oi oi-chevron-right" aria-hidden="true" /></span>
    </button>
  </section>
  <Dialog
    :visible="isMobile && !!mobilePerson"
    class="sharing-person-sheet"
    position="bottom"
    modal
    dismissableMask
    :draggable="false"
    :header="mobilePersonDetails?.name || 'Person access'"
    :pt="replaySensitiveTitlePT"
    :style="{ width: '100vw', maxWidth: '100vw' }"
    @update:visible="(value) => !value && (mobilePerson = null)"
  >
    <template v-if="mobilePersonDetails">
      <p data-sentry-mask class="muted sheet-email">{{ mobilePersonDetails.email }}</p>
      <div class="mobile-role-options" aria-label="Access role">
        <button
          v-for="role in mobilePersonDetails.roles"
          :key="role.id"
          type="button"
          :disabled="busy"
          :aria-pressed="mobilePersonDetails.role === role.id"
          @click="chooseMobileRole(role.id)"
        >
          <span
            ><strong>{{ role.name }}</strong
            ><small>{{ role.description }}</small></span
          >
          <i v-if="mobilePersonDetails.role === role.id" class="oi oi-check" aria-hidden="true" />
        </button>
      </div>
      <SharingExpiry
        v-if="mobilePerson?.kind === 'grant' && scope.kind === 'item'"
        :modelValue="grantEdit(mobilePerson.item).expiry"
        :disabled="busy || grantEdit(mobilePerson.item).role === removeRole"
        label="Access expiry"
        @update:modelValue="setMobileExpiry"
      />
      <Button
        v-if="mobilePerson?.kind === 'invitation'"
        label="Email invitation"
        severity="secondary"
        variant="text"
        :disabled="busy"
        @click="emailMobileInvitation"
      />
      <div class="sheet-remove">
        <Button
          label="Remove access"
          severity="danger"
          variant="text"
          :disabled="busy"
          @click="removeMobilePerson"
        />
      </div>
    </template>
  </Dialog>
</template>

<style scoped src="@/assets/sharing-panel.css"></style>

<style scoped>
.mobile-manage-access,
.mobile-role-options button {
  display: flex;
  align-items: center;
  gap: 16px;
  width: 100%;
  text-align: left;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  cursor: pointer;
  padding: 12px 0;
}
.mobile-role-options button > span {
  flex: 1;
}
.mobile-role-options strong,
.mobile-role-options small {
  display: block;
}
.mobile-role-options small {
  color: var(--p-text-muted-color);
  margin-top: 4px;
  font-size: 13px;
}
.mobile-manage-access {
  justify-content: space-between;
  align-items: flex-start;
  flex-wrap: wrap;
}
.mobile-manage-access > span:last-child {
  color: var(--p-primary-color);
  font-size: 14px;
}
.mobile-avatar-stack {
  display: flex;
  margin-top: 16px;
  gap: 4px;
}
.mobile-access-preview {
  border-top: 1px solid var(--p-content-border-color);
  margin-top: 24px;
  padding-top: 12px;
}
.mobile-role-options {
  display: grid;
  gap: 4px;
  margin-bottom: 16px;
}
.mobile-role-options button {
  min-height: 56px;
  border-radius: 8px;
  padding: 12px;
}
.mobile-role-options button:hover {
  background: var(--p-content-hover-background);
}
.sheet-email {
  overflow-wrap: anywhere;
  margin: 0 0 16px;
}
.sheet-remove {
  margin-top: 16px;
  padding-top: 12px;
  border-top: 1px solid var(--p-content-border-color);
}
.mobile-invitation-identity {
  flex: 1;
  min-width: 0;
}
.mobile-person-role {
  margin-top: 4px;
}
@media (max-width: 639px) {
  .access-row.mobile-person-row {
    width: 100%;
    border: 0;
    border-bottom: 1px solid var(--p-content-border-color);
    background: transparent;
    color: inherit;
    font: inherit;
    text-align: left;
    padding: 16px 0;
    flex-wrap: nowrap;
    gap: 12px;
    min-height: 72px;
    margin-inline: 0;
  }
  button.mobile-person-row {
    cursor: pointer;
  }
  .mobile-person-row .person-identity {
    flex: 1;
    min-width: 0;
  }
  .mobile-person-row:disabled {
    opacity: 0.6;
  }
  .mobile-share-content {
    margin-bottom: 8px;
  }
}
.person-row {
  flex-wrap: nowrap;
  border-radius: var(--p-border-radius-md);
  padding: 8px;
  margin-inline: -8px;
}
.person-row:hover,
.person-row:focus-within {
  background: var(--p-content-hover-background);
}
.person-row > .person-identity {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}
.person-identity > div {
  min-width: 0;
  overflow-wrap: anywhere;
}
.pending-removal .person-identity {
  opacity: 0.55;
  text-decoration: line-through;
}
.inline-role-select {
  max-width: 13rem;
  min-height: 44px;
  border-color: transparent;
  background: transparent;
  box-shadow: none;
}
.inline-role-select:deep(.p-select-label) {
  text-align: end;
}
.inline-role-select:has(:focus-visible) {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 2px;
}
@media (max-width: 480px) {
  .person-row {
    flex-wrap: wrap;
  }
  .person-row > .recipient-controls {
    margin-inline-start: auto;
  }
}
.add-person {
  display: grid;
  gap: 12px;
}
.add-person > label {
  font-weight: 500;
}
.recipient-entry,
.recipient-settings,
.recipient-identity,
.person-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}
.recipient-entry :deep(input) {
  min-width: 0;
  min-height: 44px;
  flex: 1;
}
.recipient-identity {
  flex: 1;
  overflow-wrap: anywhere;
}
.person-avatar {
  display: grid;
  place-items: center;
  flex: 0 0 36px;
  height: 36px;
  border-radius: 50%;
  background: var(--p-content-hover-background);
  color: var(--p-text-muted-color);
  font-weight: 600;
}
.role-select {
  min-width: 144px;
  min-height: 44px;
}
.recipient-controls {
  display: flex;
  align-items: center;
  gap: 4px;
}
.draft-people {
  margin: 0 0 12px;
}
.draft-person {
  padding: 8px 0;
}
.people-section {
  margin-top: 20px;
}
.people-section h3 {
  font-size: 16px;
  margin-bottom: 8px;
}
.role-label {
  color: var(--p-text-muted-color);
  font-size: 12px;
}
.past-access {
  color: var(--p-text-muted-color);
  font-size: 12px;
  margin-top: 12px;
}
summary {
  cursor: pointer;
  min-height: 44px;
  align-content: center;
}
summary:focus-visible {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 2px;
  border-radius: 4px;
}
.person-actions {
  justify-content: space-between;
  flex-wrap: wrap;
  margin-top: 20px;
}
.person-actions > .row-buttons {
  margin-inline-start: auto;
}
@media (max-width: 480px) {
  .recipient-settings {
    flex-wrap: wrap;
  }
  .recipient-controls {
    width: 100%;
  }
  .recipient-controls .role-select {
    flex: 1;
  }
  .person-row > .recipient-controls {
    width: auto;
    margin-inline-start: auto;
  }
}
</style>
