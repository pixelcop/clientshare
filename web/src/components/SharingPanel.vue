<script setup lang="ts">
import axios from 'axios';
import { Button, Checkbox, Dialog, InputText, Select } from 'openvue';

import type { AccessCapabilities, AccessInvitation, SecureLink } from '@/types/models';
import {
  describeCapabilities,
  type GrantAccess,
  type MemberAccess,
  type SharingAccess,
  type SharingScope,
} from '@/types/sharing';
import {
  sameCapabilities,
  sharingRoleLabel,
  sharingRoles,
  withinCeiling,
} from '@/types/sharingRoles';

import SharingExpiry from './SharingExpiry.vue';

const { scope } = defineProps<{ scope: SharingScope }>();
const base = computed(() => `/api/${scope.kind === 'item' ? 'files' : 'clients'}/${scope.id}`);
const key = computed(() => `${scope.kind}-${scope.id}`);
const view = ref<SharingAccess | null>(null);
const links = ref<SecureLink[]>([]);
const loading = ref(false);
const busy = ref(false);
const error = ref('');
const status = ref('');
const history = ref(false);
const activeTab = ref('people');
watch(activeTab, () => {
  error.value = '';
  status.value = '';
});
const blank = (): AccessCapabilities => ({
  can_read: false,
  can_upload: false,
  can_delete: false,
  can_share: false,
});
const rights = ref<AccessCapabilities>(blank());
const email = ref('');
type DraftPerson = {
  id: string;
  requestKey: string;
  email: string;
  role: string;
  expiry: Date | null;
};
const drafts = ref<DraftPerson[]>([]);
const scopeSummary = computed(() =>
  scope.kind === 'client'
    ? 'Entire client workspace'
    : scope.itemType === 'folder'
      ? 'This folder and everything inside'
      : 'This file only',
);
const expiry = ref<Date | null>(null);
const editing = ref<GrantAccess | null>(null);
const composing = ref(false);
function focusEmail() {
  document.getElementById(`${key.value}-email`)?.focus();
}
const editTrigger = ref<string | null>(null);
const deliveryKeys = new Map<string, string>();
const linkMode = ref('read');
const linkExpiry = ref<Date | null>(new Date(Date.now() + 7 * 86400000));
const linkKey = ref(crypto.randomUUID());
const result = ref<SecureLink | null>(null);
const recipient = ref('');
const linkEmailKey = ref(crypto.randomUUID());
const copied = ref(false);
const resultHeading = ref<HTMLElement | null>(null);
let version = 0;
let disposed = false;
const isFile = computed(() => scope.kind === 'item' && scope.itemType !== 'folder');
const roleOptions = computed(() => {
  const roles = sharingRoles(isFile.value).filter((role) =>
    withinCeiling(role.capabilities, view.value?.capabilities || blank()),
  );
  if (
    editing.value &&
    withinCeiling(rights.value, view.value?.capabilities || blank()) &&
    !roles.some((role) => sameCapabilities(role.capabilities, rights.value))
  ) {
    return [
      ...roles,
      {
        value: 'custom',
        label: 'Custom access',
        description: describeCapabilities(rights.value),
        capabilities: rights.value,
      },
    ];
  }
  return roles;
});
const selectedRole = computed({
  get: () =>
    roleOptions.value.find((role) => sameCapabilities(role.capabilities, rights.value))?.value,
  set: (value: string | undefined) => {
    const role = roleOptions.value.find((role) => role.value === value);
    if (role) {
      rights.value = { ...role.capabilities };
    }
  },
});
const roleDescription = computed(
  () => roleOptions.value.find((role) => role.value === selectedRole.value)?.description,
);
function active(item: { revoked_at?: string; expires_at?: string; accepted_at?: string }) {
  return (
    !item.revoked_at &&
    !item.accepted_at &&
    (!item.expires_at || Date.parse(item.expires_at) > Date.now())
  );
}
const pastPeople = computed(() => [
  ...(view.value?.grants || [])
    .filter((grant) => !active(grant))
    .map((grant) => ({
      id: grant.id,
      email: grant.recipient_email,
      state: grant.revoked_at ? 'Removed' : 'Expired',
    })),
  ...(view.value?.invitations || [])
    .filter((invite) => !active(invite))
    .map((invite) => ({
      id: invite.id,
      email: invite.email,
      state: invite.accepted_at
        ? 'Invitation accepted'
        : invite.revoked_at
          ? 'Invitation revoked'
          : 'Invitation expired',
    })),
]);
const modes = computed(() =>
  [
    { label: 'View & download', value: 'read', allowed: view.value?.capabilities.can_read },
    {
      label: 'Upload only',
      value: 'write',
      allowed: !isFile.value && view.value?.capabilities.can_upload,
    },
    {
      label: 'View, download & upload',
      value: 'readwrite',
      allowed:
        !isFile.value && view.value?.capabilities.can_read && view.value?.capabilities.can_upload,
    },
  ].filter((mode) => mode.allowed),
);
const visibleGrants = computed(() => (view.value?.grants || []).filter(active));
const visibleInvitations = computed(() => (view.value?.invitations || []).filter(active));
const visibleLinks = computed(() => links.value.filter((link) => history.value || active(link)));
function date(value?: string) {
  if (!value) {
    return 'No expiry';
  }
  const days = Math.ceil((Date.parse(value) - Date.now()) / 86400000);
  if (days > 0 && days <= 30) {
    return `in ${days} ${days === 1 ? 'day' : 'days'}`;
  }
  return new Date(value).toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  });
}
function expirySummary(value: Date | null) {
  return value ? `Expires ${date(value.toISOString())}` : 'No expiry';
}
function failure(e: unknown, fallback: string) {
  return axios.isAxiosError(e) && typeof e.response?.data?.error === 'string'
    ? e.response.data.error
    : fallback;
}
function resetForm() {
  composing.value = false;
  editing.value = null;
  email.value = '';
  drafts.value = [];
  expiry.value = null;
  rights.value = { ...(roleOptions.value[0]?.capabilities || blank()) };
}
async function addPerson() {
  if (!email.value.trim() || !selectedRole.value || busy.value) {
    return;
  }
  const address = email.value.trim();
  if (drafts.value.some((person) => person.email.toLowerCase() === address.toLowerCase())) {
    error.value = 'This person is already in your invitation list.';
    return;
  }
  drafts.value.push({
    id: crypto.randomUUID(),
    requestKey: crypto.randomUUID(),
    email: address,
    role: roleOptions.value[0]!.value,
    expiry: null,
  });
  email.value = '';
  error.value = '';
  status.value = '';
  editTrigger.value = null;
  composing.value = true;
  await nextTick();
  focusEmail();
}
function removeDraft(id: string) {
  drafts.value = drafts.value.filter((person) => person.id !== id);
  composing.value = drafts.value.length > 0;
  void nextTick(focusEmail);
}
function updateDraft(person: DraftPerson) {
  person.requestKey = crypto.randomUUID();
}
async function backToPeople(keepEmail = false) {
  const trigger = editTrigger.value;
  const draft = !editing.value && keepEmail ? email.value : '';
  resetForm();
  email.value = draft;
  await nextTick();
  if (trigger && document.getElementById(trigger)) {
    document.getElementById(trigger)?.focus();
  } else {
    focusEmail();
  }
}
async function load() {
  const current = ++version;
  loading.value = true;
  view.value = null;
  links.value = [];
  error.value = '';
  try {
    const [access, list] = await Promise.all([
      axios.get<SharingAccess>(`${base.value}/access`),
      axios.get<SecureLink[]>(`${base.value}/links`),
    ]);
    if (current !== version) {
      return;
    }
    view.value = access.data;
    links.value = list.data;
    if (!editing.value && !drafts.value.length) {
      resetForm();
    }
    if (!modes.value.some((mode) => mode.value === linkMode.value)) {
      linkMode.value = modes.value[0]?.value || '';
    }
  } catch (e) {
    if (current === version) {
      error.value = failure(e, 'Access settings could not be loaded.');
    }
  } finally {
    if (current === version) {
      loading.value = false;
    }
  }
}
watch(
  key,
  () => {
    resetForm();
    editTrigger.value = null;
    result.value = null;
    recipient.value = '';
    status.value = '';
    linkMode.value = 'read';
    linkExpiry.value = new Date(Date.now() + 7 * 86400000);
    linkKey.value = crypto.randomUUID();
    history.value = false;
    activeTab.value = 'people';
    deliveryKeys.clear();
    void load();
  },
  { immediate: true },
);
onBeforeUnmount(() => {
  version += 1;
  disposed = true;
});
watch([linkMode, linkExpiry], () => {
  linkKey.value = crypto.randomUUID();
});
watch(recipient, () => {
  linkEmailKey.value = crypto.randomUUID();
});
async function sendInvitation(invitation: AccessInvitation) {
  let request = deliveryKeys.get(invitation.id);
  if (!request) {
    request = crypto.randomUUID();
    deliveryKeys.set(invitation.id, request);
  }
  try {
    const response = await axios.post<{ status: string }>(
      `/api/invitations/${invitation.id}/email`,
      {},
      { headers: { 'Idempotency-Key': request } },
    );
    status.value =
      response.data.status === 'sent'
        ? 'Invitation email sent.'
        : 'Invitation email is waiting to send.';
  } catch (e) {
    status.value = `Invitation saved. ${failure(e, 'We could not confirm the email was sent. Try Email invitation again.')}`;
  }
}
async function sendPeople() {
  if (busy.value || !drafts.value.length || email.value.trim()) {
    return;
  }
  busy.value = true;
  error.value = '';
  status.value = '';
  const messages: string[] = [];
  const target = key.value;
  const endpoint = base.value;
  try {
    for (const person of [...drafts.value]) {
      if (disposed || target !== key.value) {
        return;
      }
      const role = roleOptions.value.find((option) => option.value === person.role);
      if (!role || !view.value || !withinCeiling(role.capabilities, view.value.capabilities)) {
        throw new Error('Invalid role');
      }
      const response = await axios.post<AccessInvitation>(
        `${endpoint}/invitations`,
        {
          email: person.email,
          capabilities: role.capabilities,
          expires_at: person.expiry?.toISOString() || null,
        },
        { headers: { 'Idempotency-Key': person.requestKey } },
      );
      await sendInvitation(response.data);
      if (disposed || target !== key.value) {
        return;
      }
      messages.push(`${person.email}: ${status.value}`);
      drafts.value = drafts.value.filter((draft) => draft.id !== person.id);
    }
    resetForm();
    await load();
  } catch (e) {
    if (disposed || target !== key.value) {
      return;
    }
    if (messages.length) {
      await load();
    }
    error.value = failure(e, 'Some invitations could not be saved. Retry the remaining people.');
  } finally {
    if (!disposed && target === key.value) {
      status.value = messages.join(' ');
      busy.value = false;
      await nextTick();
      focusEmail();
    }
  }
}
async function savePerson() {
  if (
    !view.value ||
    busy.value ||
    !composing.value ||
    !editing.value ||
    !selectedRole.value ||
    !withinCeiling(rights.value, view.value.capabilities)
  ) {
    return;
  }
  const target = key.value;
  busy.value = true;
  error.value = '';
  status.value = '';
  try {
    const settings = {
      capabilities: { ...rights.value },
      expires_at: expiry.value?.toISOString() || null,
    };
    await axios.patch(`${base.value}/grants/${editing.value.id}`, {
      ...settings,
      version: editing.value.version,
      user_id: editing.value.user_id,
    });
    status.value = 'Access updated. Access through the client or another folder is unchanged.';
    if (target !== key.value) {
      return;
    }
    resetForm();
    await load();
  } catch (e) {
    if (target === key.value) {
      error.value = failure(e, 'Access could not be saved. Refresh before retrying.');
    }
  } finally {
    busy.value = false;
    if (target === key.value && !composing.value && view.value) {
      await nextTick();
      focusEmail();
    }
  }
}
async function edit(grant: GrantAccess) {
  error.value = '';
  status.value = '';
  editTrigger.value = `${key.value}-grant-${grant.id}`;
  composing.value = true;
  editing.value = grant;
  email.value = grant.recipient_email;
  expiry.value = grant.expires_at ? new Date(grant.expires_at) : null;
  rights.value = {
    can_read: grant.can_read,
    can_upload: grant.can_upload,
    can_delete: grant.can_delete,
    can_share: grant.can_share,
  };
  await nextTick();
  document.getElementById(`${key.value}-role`)?.focus();
}
function canEdit(grant: GrantAccess) {
  const ceiling = view.value?.capabilities;
  return (
    ceiling &&
    (!grant.can_read || ceiling.can_read) &&
    (!grant.can_upload || ceiling.can_upload) &&
    (!grant.can_delete || ceiling.can_delete) &&
    (!grant.can_share || ceiling.can_share)
  );
}
async function createLink() {
  if (!linkExpiry.value) {
    return;
  }
  busy.value = true;
  error.value = '';
  status.value = '';
  try {
    const response = await axios.post<SecureLink>(
      `${base.value}/links`,
      { access_type: linkMode.value, expiry_days: 7, expires_at: linkExpiry.value.toISOString() },
      { headers: { 'Idempotency-Key': linkKey.value } },
    );
    result.value = response.data;
    recipient.value = '';
    copied.value = false;
    await load();
    await nextTick();
    resultHeading.value?.focus();
  } catch (e) {
    error.value = failure(e, 'The link could not be created. Please try again.');
  } finally {
    busy.value = false;
  }
}
function createAnotherLink() {
  result.value = null;
  linkKey.value = crypto.randomUUID();
}
function useLink(link: SecureLink) {
  result.value = link;
  recipient.value = '';
  linkEmailKey.value = crypto.randomUUID();
  copied.value = false;
  status.value = '';
  error.value = '';
}
async function copy(link: SecureLink) {
  try {
    await navigator.clipboard.writeText(link.link_url || '');
    copied.value = true;
  } catch {
    status.value = 'Could not copy. Select and copy the link address.';
  }
}
async function emailLink() {
  if (!result.value) {
    return;
  }
  busy.value = true;
  error.value = '';
  try {
    const response = await axios.post<{ status: string }>(
      `/api/links/${result.value.id}/email`,
      { email: recipient.value },
      { headers: { 'Idempotency-Key': linkEmailKey.value } },
    );
    status.value =
      response.data.status === 'sent' ? 'Link email sent.' : 'Link email is waiting to send.';
  } catch (e) {
    error.value = failure(
      e,
      'We could not confirm the email was sent. Try again or copy the link.',
    );
  } finally {
    busy.value = false;
  }
}
type Removal =
  | { kind: 'grant'; item: GrantAccess }
  | { kind: 'invitation'; item: AccessInvitation }
  | { kind: 'member'; item: MemberAccess }
  | { kind: 'link'; item: SecureLink };
const removal = ref<Removal | null>(null);
const revokeLinks = ref(false);
function confirmRemove(value: Removal) {
  revokeLinks.value = false;
  removal.value = value;
}
async function remove() {
  const selected = removal.value;
  if (!selected) {
    return;
  }
  busy.value = true;
  error.value = '';
  try {
    if (selected.kind === 'grant') {
      await axios.delete(`${base.value}/grants/${selected.item.id}`, {
        data: { version: selected.item.version },
      });
    }
    if (selected.kind === 'invitation') {
      await axios.delete(`/api/invitations/${selected.item.id}`);
    }
    if (selected.kind === 'member') {
      await axios.delete(`${base.value}/members/${selected.item.user_id}`, {
        data: { revoke_links: revokeLinks.value },
      });
    }
    if (selected.kind === 'link') {
      await axios.delete(`/api/links/${selected.item.id}`);
      if (result.value?.id === selected.item.id) {
        result.value = null;
      }
    }
    removal.value = null;
    resetForm();
    status.value = 'Access removed.';
    await load();
  } catch (e) {
    error.value = failure(e, 'Access could not be removed. Refresh before retrying.');
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="sharing-panel" :class="{ 'is-composing': composing }">
    <p v-if="scope.kind === 'client'" class="scope-label muted">
      {{ scope.kind === 'client' ? 'Entire client workspace · all files and folders' : '' }}
    </p>
    <p v-if="loading" role="status">Loading access…</p>
    <div v-if="error" role="alert">
      <p>{{ error }}</p>
      <Button label="Refresh access" severity="secondary" :disabled="busy" @click="load" />
    </div>
    <p v-if="status" role="status" class="delivery-status">{{ status }}</p>
    <template v-if="view && !loading">
      <div v-if="activeTab === 'people'">
        <div v-if="drafts.length" class="draft-people" aria-label="People to invite">
          <div v-for="person in drafts" :key="person.id" class="draft-person">
            <div class="recipient-settings">
              <div class="recipient-identity">
                <span class="person-avatar" aria-hidden="true">{{
                  person.email[0]?.toUpperCase()
                }}</span>
                <strong>{{ person.email }}</strong>
              </div>
              <div class="recipient-controls">
                <Select
                  v-model="person.role"
                  :ariaLabel="`Role for ${person.email}`"
                  :options="roleOptions"
                  optionLabel="label"
                  optionValue="value"
                  :disabled="busy"
                  class="role-select"
                  :labelStyle="{ display: 'flex', alignItems: 'center', textAlign: 'start' }"
                  @update:modelValue="updateDraft(person)"
                >
                  <template #option="{ option }"
                    ><div class="role-option">
                      <strong>{{ option.label }}</strong
                      ><small>{{ option.description }}</small>
                    </div></template
                  >
                </Select>
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
        <form v-if="!editing" class="add-person" @submit.prevent="addPerson">
          <div class="recipient-entry">
            <InputText
              :id="`${key}-email`"
              v-model="email"
              type="email"
              autocomplete="email"
              :placeholder="drafts.length ? 'Add another email address' : 'Enter an email address'"
              autofocus
              required
              :disabled="busy || !roleOptions.length"
              fluid
            />
            <Button
              v-if="email.trim()"
              type="submit"
              label="Add"
              :disabled="busy || !selectedRole"
            />
          </div>
          <p v-if="!roleOptions.length" class="muted">
            You don’t have a shareable role for this item.
          </p>
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
        <form v-if="editing" class="person-form" @submit.prevent="savePerson">
          <Button
            label="Back"
            severity="secondary"
            variant="text"
            class="back-button"
            :disabled="busy"
            @click="backToPeople(true)"
          />
          <div class="recipient-settings">
            <div class="recipient-identity">
              <span class="person-avatar" aria-hidden="true">{{ email[0]?.toUpperCase() }}</span
              ><strong>{{ email }}</strong>
            </div>
            <Select
              v-model="selectedRole"
              :inputId="`${key}-role`"
              ariaLabel="Role"
              :options="roleOptions"
              optionLabel="label"
              optionValue="value"
              :disabled="busy"
              class="role-select"
              :labelStyle="{ display: 'flex', alignItems: 'center', textAlign: 'start' }"
            >
              <template #option="{ option }"
                ><div class="role-option">
                  <strong>{{ option.label }}</strong
                  ><small>{{ option.description }}</small>
                </div></template
              >
            </Select>
          </div>
          <p class="muted">{{ roleDescription }}</p>
          <SharingExpiry v-if="scope.kind === 'item'" v-model="expiry" :disabled="busy" />
          <p class="muted">{{ scopeSummary }} · {{ expirySummary(expiry) }}</p>
          <p v-if="editing" class="muted">
            Changes apply to access added here. Access through the client or another folder is
            unchanged.
          </p>
          <p v-else class="muted">An invitation will be emailed to this address.</p>
          <div class="person-actions">
            <Button
              v-if="editing"
              label="Remove access"
              severity="danger"
              variant="text"
              :disabled="busy"
              @click="confirmRemove({ kind: 'grant', item: editing })"
            />
            <div class="row-buttons">
              <Button
                label="Cancel"
                severity="secondary"
                variant="text"
                :disabled="busy"
                @click="backToPeople()"
              /><Button
                type="submit"
                :label="editing ? 'Save changes' : 'Send invitation'"
                :loading="busy"
                :disabled="!selectedRole"
              />
            </div>
          </div>
        </form>
        <section v-if="!composing" aria-label="People with access" class="people-section">
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
          <div v-for="person in view.people" :key="person.user_id" class="access-row">
            <div>
              <strong>{{ person.name || person.email }}</strong
              ><small>{{ person.email }}</small>
            </div>
            <div class="row-buttons">
              <span class="role-label" :title="describeCapabilities(person.capabilities)">{{
                sharingRoleLabel(person.capabilities, isFile)
              }}</span
              ><Button
                v-if="person.can_remove"
                label="Remove"
                severity="secondary"
                variant="text"
                :disabled="busy"
                @click="confirmRemove({ kind: 'member', item: person })"
              />
            </div>
          </div>
          <div v-for="grant in visibleGrants" :key="grant.id" class="access-row">
            <div>
              <strong>{{ grant.recipient_name || grant.recipient_email }}</strong
              ><small
                >{{ grant.recipient_email
                }}<template v-if="grant.expires_at">
                  · Expires {{ date(grant.expires_at) }}</template
                ></small
              >
            </div>
            <div class="row-buttons">
              <Button
                v-if="canEdit(grant)"
                :id="`${key}-grant-${grant.id}`"
                :label="sharingRoleLabel(grant, isFile)"
                :aria-label="`Change access for ${grant.recipient_email}`"
                :aria-description="`Current role: ${sharingRoleLabel(grant, isFile)}`"
                icon="oi oi-chevron-down"
                iconPos="right"
                severity="secondary"
                :disabled="busy"
                variant="text"
                @click="edit(grant)"
              /><span v-else class="role-label" :title="describeCapabilities(grant)">{{
                sharingRoleLabel(grant, isFile)
              }}</span>
            </div>
          </div>
          <div v-for="invitation in visibleInvitations" :key="invitation.id" class="access-row">
            <div>
              <strong>{{ invitation.email }}</strong
              ><small
                >{{
                  invitation.accepted_at
                    ? 'Accepted'
                    : invitation.revoked_at
                      ? 'Removed'
                      : active(invitation)
                        ? 'Awaiting acceptance'
                        : 'Expired'
                }}
                · {{ sharingRoleLabel(invitation, isFile)
                }}<template v-if="invitation.grant_expires_at">
                  · Access expires {{ date(invitation.grant_expires_at) }}</template
                ></small
              >
            </div>
            <details v-if="active(invitation)" class="invitation-actions">
              <summary :aria-label="`Invitation options for ${invitation.email}`">Pending</summary>
              <div class="row-buttons">
                <Button
                  label="Email invitation"
                  severity="secondary"
                  variant="text"
                  :disabled="busy"
                  @click="sendInvitation(invitation)"
                /><Button
                  label="Revoke"
                  severity="secondary"
                  variant="text"
                  :disabled="busy"
                  @click="confirmRemove({ kind: 'invitation', item: invitation })"
                />
              </div>
            </details>
          </div>
          <details v-if="pastPeople.length" class="past-access">
            <summary>Past invitations & access ({{ pastPeople.length }})</summary>
            <div v-for="person in pastPeople" :key="person.id" class="access-row">
              <div>
                <strong>{{ person.email }}</strong
                ><small>{{ person.state }}</small>
              </div>
            </div>
          </details>
        </section>
      </div>
      <div v-else>
        <Button
          label="Back to sharing"
          icon="oi oi-arrow-left"
          severity="secondary"
          variant="text"
          :disabled="busy"
          @click="activeTab = 'people'"
        />
        <section aria-label="Sharing links">
          <label class="history-toggle"
            ><Checkbox v-model="history" binary :inputId="`${key}-history`" /> Include expired and
            revoked links</label
          >
          <p class="muted">
            Anyone with a link gets the permissions you choose. Removing a person won’t stop them
            using a link they already have.
          </p>
          <div v-for="link in visibleLinks" :key="link.id" class="access-row">
            <div>
              <strong>{{
                link.access_type === 'write'
                  ? 'Upload only'
                  : link.access_type === 'readwrite'
                    ? 'View, download & upload'
                    : 'View & download'
              }}</strong
                ><small>{{ link.revoked_at ? 'Revoked' : `Expires ${date(link.expires_at)}` }}</small>
            </div>
            <div class="row-buttons">
              <Button
                v-if="active(link) && link.link_url"
                label="Use link"
                severity="secondary"
                :disabled="busy"
                @click="useLink(link)"
              /><Button
                v-if="active(link)"
                label="Revoke"
                severity="secondary"
                :disabled="busy"
                @click="confirmRemove({ kind: 'link', item: link })"
              />
            </div>
          </div>
          <form v-if="!result && modes.length" class="access-form" @submit.prevent="createLink">
            <h4>Create a link for {{ scope.name }}</h4>
            <label :id="`${key}-mode-label`" :for="`${key}-mode`">Anyone with this link can</label
            ><Select
              v-model="linkMode"
              :inputId="`${key}-mode`"
              :ariaLabelledby="`${key}-mode-label`"
              :options="modes"
              optionLabel="label"
              optionValue="value"
              :disabled="busy"
              fluid
            />
            <SharingExpiry v-model="linkExpiry" :disabled="busy" compact required />
            <p class="muted">
              {{ scope.name }} · {{ modes.find((mode) => mode.value === linkMode)?.label }} · Anyone
              with the link · {{ expirySummary(linkExpiry) }}
            </p>
            <Button
              type="submit"
              label="Create link"
              :loading="busy"
              :disabled="!linkMode || !linkExpiry"
            />
          </form>
          <div v-if="result && active(result)" class="access-form" aria-label="Created link">
            <h4 ref="resultHeading" tabindex="-1">Link ready: {{ scope.name }}</h4>
            <p class="muted">
              {{ modes.find((mode) => mode.value === result?.access_type)?.label }} ·
              {{ scopeSummary }} · Expires {{ date(result.expires_at) }}
            </p>
            <p class="muted">Copy the link to share it yourself, or send it by email below.</p>
            <label :for="`${key}-url`">Link address</label
            ><InputText :id="`${key}-url`" :modelValue="result.link_url" readonly fluid />
            <div class="row-buttons">
              <Button
                :label="copied ? 'Copied' : 'Copy link'"
                :disabled="!result.link_url"
                @click="copy(result)"
              /><Button
                as="a"
                :href="result.link_url"
                target="_blank"
                rel="noopener noreferrer"
                label="Open recipient view"
                severity="secondary"
              />
            </div>
            <form class="access-form" @submit.prevent="emailLink">
              <label :for="`${key}-link-email`">Email link to</label
              ><InputText
                :id="`${key}-link-email`"
                v-model="recipient"
                type="email"
                required
                fluid
              /><Button type="submit" label="Email link" :loading="busy" />
            </form>
            <Button
              label="Create another link"
              severity="secondary"
              :disabled="busy"
              @click="createAnotherLink"
            />
          </div>
          <router-link
            v-if="scope.kind === 'client' && view.can_manage"
            :to="`/clients/${scope.id}/links`"
            class="management-link"
            >Manage links</router-link
          >
        </section>
      </div>
      <div v-if="!composing" class="sharing-footer">
        <Button
          v-if="activeTab === 'people'"
          label="Sharing links"
          icon="oi oi-link"
          severity="secondary"
          variant="text"
          @click="activeTab = 'links'"
        />
        <span v-else />
        <slot name="done" />
      </div>
    </template>
    <Dialog
      :visible="!!removal"
      header="Remove access?"
      modal
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
      <label v-if="removal?.kind === 'member'" class="history-toggle"
        ><Checkbox v-model="revokeLinks" binary :inputId="`${key}-revoke-links`" /> Also turn off
        every sharing link for this client</label
      >
      <p v-if="error" role="alert">{{ error }}</p>
      <template #footer
        ><Button
          label="Cancel"
          severity="secondary"
          :disabled="busy"
          @click="removal = null" /><Button
          label="Remove access"
          severity="danger"
          :loading="busy"
          @click="remove"
      /></template>
    </Dialog>
  </div>
</template>

<style scoped>
.sharing-panel {
  display: grid;
  gap: 8px;
  color: var(--p-text-color);
  font-size: 14px;
}
.scope-label,
h3,
h4 {
  font-weight: 600;
  overflow-wrap: anywhere;
}
.scope-label {
  font-size: 12px;
  font-weight: 400;
}
h3 {
  font-size: 18px;
  margin-bottom: 12px;
}
h4 {
  margin-bottom: 4px;
}
.muted,
small {
  color: var(--p-text-muted-color);
  line-height: 1.5;
}
section {
  min-width: 0;
}
.access-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 0;
}
.access-row > div:first-child {
  flex: 1;
  min-width: 160px;
  overflow-wrap: anywhere;
}
strong,
small {
  display: block;
}
small {
  font-size: 12px;
  margin-top: 4px;
}
.row-buttons,
.history-toggle {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}
.access-form {
  display: grid;
  gap: 12px;
  margin-top: 20px;
}
.history-toggle {
  min-height: 44px;
  margin-top: 12px;
}
.sharing-panel :deep(.p-button) {
  min-height: 44px;
}
.delivery-status {
  padding: 12px;
  border-inline-start: 3px solid var(--p-primary-color);
}
.add-person,
.person-form {
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
.role-option {
  max-width: min(320px, 70vw);
  white-space: normal;
}
.role-option small {
  display: block;
  margin-top: 4px;
  font-size: 12px;
  color: var(--p-text-muted-color);
}
.back-button {
  justify-self: start;
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
.inheritance-note {
  margin-top: 16px;
  font-size: 12px;
}
.past-access {
  color: var(--p-text-muted-color);
  font-size: 12px;
}
.past-access {
  margin-top: 12px;
}
.management-link {
  display: inline-flex;
  align-items: center;
  min-height: 44px;
  margin-top: 12px;
  color: var(--p-primary-color);
  text-decoration: underline;
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
}
</style>
