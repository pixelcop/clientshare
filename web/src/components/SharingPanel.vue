<script setup lang="ts">
import axios from 'axios';
import { Button, Checkbox, Dialog, InputText } from 'openvue';

import AccessSelect from '@/components/AccessSelect.vue';
import type { TenantAccessRole } from '@/types/accessRoles';
import {
  applicableRole,
  defaultAccessRole,
  roleCapabilities,
  roleSummary,
  tenantRoleLabel,
} from '@/types/accessRoles';
import type { AccessCapabilities, AccessInvitation, SecureLink } from '@/types/models';
import {
  describeCapabilities,
  type GrantAccess,
  type MemberAccess,
  type SharingAccess,
  type SharingScope,
} from '@/types/sharing';
import { withinCeiling } from '@/types/sharingRoles';

import ScopedGroupAccess from './ScopedGroupAccess.vue';
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
const recipientType = ref<'internal' | 'external'>('external');
type DraftPerson = {
  id: string;
  requestKey: string;
  email: string;
  role: string;
  userType: 'internal' | 'external';
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
const memberEdits = ref<Record<string, string>>({});
function memberRoles(person: MemberAccess) {
  return catalog.value.filter(
    (r) =>
      (applicableRole(r, person.user_type || 'external', 'workspace') || r.id === person.role_id) &&
      withinCeiling(roleCapabilities(r), view.value?.capabilities || blank()),
  );
}
async function saveMember(person: MemberAccess) {
  if (!memberEdits.value[person.user_id] || busy.value) {
    return;
  }
  busy.value = true;
  error.value = '';
  try {
    await axios.put(`${base.value}/members/${person.user_id}/role`, {
      role_id: memberEdits.value[person.user_id],
      expected_role_id: person.role_id || '',
    });
    status.value = 'Workspace role updated. Other access is unchanged.';
    await load();
  } catch (e) {
    error.value = failure(e, 'Workspace role could not be updated.');
  } finally {
    busy.value = false;
  }
}
const linkEmailKey = ref(crypto.randomUUID());
const copied = ref(false);
const resultHeading = ref<HTMLElement | null>(null);
let version = 0;
let disposed = false;
const isFile = computed(() => scope.kind === 'item' && scope.itemType !== 'folder');
const catalog = ref<TenantAccessRole[]>([]);
const selectedRoleId = ref('');
const roleLabel = (id?: string) => tenantRoleLabel(catalog.value, id);
const canInvite = computed(() =>
  view.value?.capabilities.sharing
    ? view.value.capabilities.sharing.invite_external ||
      view.value.capabilities.sharing.manage_access
    : view.value?.capabilities.can_share,
);
const canCreateLinks = computed(
  () => view.value?.capabilities.sharing?.create_links ?? view.value?.capabilities.can_share,
);
const roleOptions = computed(() => {
  const kind = editing.value?.recipient_user_type || recipientType.value;
  const target = isFile.value ? 'file' : scope.kind === 'client' ? 'workspace' : 'folder';
  return catalog.value
    .filter(
      (r) =>
        (applicableRole(r, kind, target) || (r.id === editing.value?.role_id && r.archived_at)) &&
        withinCeiling(roleCapabilities(r, isFile.value), view.value?.capabilities || blank()),
    )
    .map((r) => ({
      value: r.id,
      label: `${r.name}${r.archived_at ? ' (archived)' : ''}`,
      description: roleSummary(r, isFile.value),
      capabilities: roleCapabilities(r, isFile.value),
    }));
});
const selectedRole = computed({
  get: () => selectedRoleId.value,
  set: (value: string | undefined) => {
    const role = roleOptions.value.find((role) => role.value === value);
    if (role) {
      selectedRoleId.value = role.value;
      rights.value = { ...role.capabilities };
    }
  },
});
const roleDescription = computed(
  () => roleOptions.value.find((role) => role.value === selectedRole.value)?.description,
);
watch(recipientType, () => {
  selectedRoleId.value = defaultAccessRole(
    catalog.value.filter((r) => roleOptions.value.some((o) => o.value === r.id)),
  );
  rights.value =
    roleOptions.value.find((r) => r.value === selectedRoleId.value)?.capabilities || blank();
});
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
    {
      label: 'View & download',
      value: 'read',
      description: 'Open and download documents. Cannot upload or delete.',
      allowed: view.value?.capabilities.can_read,
    },
    {
      label: 'Upload only',
      value: 'write',
      description: 'Upload documents without viewing existing files.',
      allowed: !isFile.value && view.value?.capabilities.can_upload,
    },
    {
      label: 'View, download & upload',
      value: 'readwrite',
      description: 'Open, download and upload documents. Cannot delete.',
      allowed:
        !isFile.value && view.value?.capabilities.can_read && view.value?.capabilities.can_upload,
    },
  ].filter((mode) => mode.allowed),
);
const visibleGrants = computed(() => (view.value?.grants || []).filter(active));
const visibleInvitations = computed(() => (view.value?.invitations || []).filter(active));
const hasLinkHistory = computed(() => links.value.some((link) => !active(link)));
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
  selectedRoleId.value = defaultAccessRole(
    catalog.value.filter((r) => roleOptions.value.some((o) => o.value === r.id)),
  );
  const safe = roleOptions.value.find((r) => r.value === selectedRoleId.value);
  rights.value = safe?.capabilities || blank();
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
    userType: recipientType.value,
    role: selectedRole.value,
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
    const [access, roles] = await Promise.all([
      axios.get<SharingAccess>(`${base.value}/access`),
      axios.get<TenantAccessRole[]>('/api/access-roles'),
    ]);
    if (current !== version) {
      return;
    }
    view.value = access.data;
    catalog.value = roles.data;
    const missing = [
      ...(access.data.grants || []),
      ...(access.data.invitations || []),
      ...(access.data.people || []),
      ...(access.data.additional_access || []),
    ]
      .map((g) => g.role_id)
      .filter((id): id is string => !!id && !catalog.value.some((r) => r.id === id));
    const preserved = await Promise.all(
      [...new Set(missing)].map((id) => axios.get<TenantAccessRole>(`/api/access-roles/${id}`)),
    );
    if (current !== version) {
      return;
    }
    catalog.value.push(...preserved.map((r) => r.data));
    const canLinks =
      access.data.capabilities.sharing?.create_links ?? access.data.capabilities.can_share;
    links.value = canLinks ? (await axios.get<SecureLink[]>(`${base.value}/links`)).data : [];
    if (current !== version) {
      return;
    }
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
      const tenantRole = catalog.value.find(
        (r) =>
          r.id === person.role &&
          applicableRole(
            r,
            person.userType,
            isFile.value ? 'file' : scope.kind === 'client' ? 'workspace' : 'folder',
          ),
      );
      const role = tenantRole ? { capabilities: roleCapabilities(tenantRole, isFile.value) } : null;
      if (!role || !view.value || !withinCeiling(role.capabilities, view.value.capabilities)) {
        throw new Error('Invalid role');
      }
      const response = await axios.post<AccessInvitation>(
        `${endpoint}/invitations`,
        {
          email: person.email,
          user_type: person.userType,
          role_id: person.role,
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
      role_id: selectedRole.value,
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
  selectedRoleId.value = grant.role_id || '';
  email.value = grant.recipient_email;
  expiry.value = grant.expires_at ? new Date(grant.expires_at) : null;
  rights.value = {
    can_read: grant.can_read,
    can_upload: grant.can_upload,
    can_delete: grant.can_delete,
    can_share: grant.can_share,
    sharing: grant.sharing,
  };
  await nextTick();
  document.getElementById(`${key.value}-role`)?.focus();
}
function canEdit(grant: GrantAccess) {
  const ceiling = view.value?.capabilities;
  return ceiling && grant.can_manage !== false && withinCeiling(grant, ceiling);
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
      <p v-if="view.inherits_client_access" class="inheritance-note">
        Access also comes from the client workspace, including any workspace group assignments.
        Removing access added here does not remove that inherited access.
      </p>
      <p v-if="view.inherits_folder_access" class="inheritance-note">
        Access also comes from a containing folder. Change the assignment at that folder to remove
        its inherited access.
      </p>
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
        <form v-if="!editing && canInvite" class="add-person" @submit.prevent="addPerson">
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
          <div
            v-if="
              !drafts.length &&
              (view.capabilities.sharing?.manage_access ?? view.capabilities.can_share)
            "
            class="grid gap-2 mb-3"
          >
            <label :id="`${key}-recipient-type-label`">Recipient type</label>
            <AccessSelect
              v-model="recipientType"
              :ariaLabelledby="`${key}-recipient-type-label`"
              :options="[
                {
                  label: 'External contact',
                  value: 'external',
                  description: 'Invite a client or adviser with the selected document role.',
                },
                {
                  label: 'Existing internal team member',
                  value: 'internal',
                  description: 'Assign access to someone already on your team.',
                },
              ]"
              optionLabel="label"
              optionValue="value"
              :disabled="busy"
            />
            <p v-if="recipientType === 'internal'" class="muted">
              Use an existing team member’s email. Create new internal accounts from Users first.
            </p>
          </div>
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
            <AccessSelect
              v-model="selectedRole"
              :inputId="`${key}-role`"
              ariaLabel="Role"
              :options="roleOptions"
              optionLabel="label"
              optionValue="value"
              :disabled="busy"
              class="role-select"
              :labelStyle="{ display: 'flex', alignItems: 'center', textAlign: 'start' }"
            />
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
          <ScopedGroupAccess
            :key="key"
            :scope="scope"
            :clientId="view.client_id || (scope.kind === 'client' ? scope.id : '')"
            :roles="
              catalog.filter((r) => withinCeiling(roleCapabilities(r, isFile), view!.capabilities))
            "
            :assignments="view.additional_access || []"
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
          <div v-for="person in view.people" :key="person.user_id" class="access-row">
            <div>
              <strong>{{ person.name || person.email }}</strong
              ><small>{{ person.email }}</small>
            </div>
            <div class="row-buttons">
              <span class="role-label" :title="describeCapabilities(person.capabilities)">{{
                roleLabel(person.role_id)
              }}</span
              ><template v-if="person.can_remove"
                ><AccessSelect
                  v-model="memberEdits[person.user_id]"
                  :options="memberRoles(person)"
                  optionLabel="name"
                  optionValue="id"
                  :ariaLabel="`Workspace role for ${person.email}`"
                  :placeholder="roleLabel(person.role_id)"
                  :disabled="busy" /><Button
                  label="Save role"
                  severity="secondary"
                  :disabled="busy || !memberEdits[person.user_id]"
                  @click="saveMember(person)" /></template
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
                :label="roleLabel(grant.role_id)"
                :aria-label="`Change access for ${grant.recipient_email}`"
                :aria-description="`Current role: ${roleLabel(grant.role_id)}`"
                icon="oi oi-chevron-down"
                iconPos="right"
                severity="secondary"
                :disabled="busy"
                variant="text"
                @click="edit(grant)"
              /><span v-else class="role-label" :title="describeCapabilities(grant)">{{
                roleLabel(grant.role_id)
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
                · {{ roleLabel(invitation.role_id)
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
              </div>
              <div class="row-buttons">
                <Button
                  v-if="active(link) && link.link_url"
                  label="Use link"
                  severity="secondary"
                  :disabled="busy"
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
          </section>
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
          v-if="activeTab === 'people' && canCreateLinks"
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
  .link-settings {
    grid-template-columns: minmax(0, 1fr);
  }
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
