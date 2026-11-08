<script setup lang="ts">
import axios from 'axios';
import { Button, Checkbox, DatePicker, Dialog, InputText, Select } from 'openvue';

import type { AccessCapabilities, AccessInvitation, SecureLink } from '@/types/models';
import {
  describeCapabilities,
  type GrantAccess,
  type MemberAccess,
  type SharingAccess,
  type SharingScope,
} from '@/types/sharing';

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
const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
const blank = (): AccessCapabilities => ({
  can_read: false,
  can_upload: false,
  can_delete: false,
  can_share: false,
});
const rights = ref<AccessCapabilities>(blank());
const email = ref('');
const expiry = ref<Date | null>(null);
const editing = ref<GrantAccess | null>(null);
const creationKey = ref(crypto.randomUUID());
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
const isFile = computed(() => scope.kind === 'item' && scope.itemType !== 'folder');
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
function active(item: { revoked_at?: string; expires_at?: string; accepted_at?: string }) {
  return (
    !item.revoked_at &&
    !item.accepted_at &&
    (!item.expires_at || Date.parse(item.expires_at) > Date.now())
  );
}
const visibleGrants = computed(() =>
  (view.value?.grants || []).filter((grant) => history.value || active(grant)),
);
const visibleInvitations = computed(() =>
  (view.value?.invitations || []).filter((invite) => history.value || active(invite)),
);
const visibleLinks = computed(() => links.value.filter((link) => history.value || active(link)));
function date(value?: string) {
  return value ? new Date(value).toLocaleString(undefined, { timeZoneName: 'short' }) : 'No expiry';
}
function failure(e: unknown, fallback: string) {
  return axios.isAxiosError(e) && typeof e.response?.data?.error === 'string'
    ? e.response.data.error
    : fallback;
}
function resetForm() {
  editing.value = null;
  email.value = '';
  expiry.value = null;
  rights.value = { ...blank(), can_read: !!view.value?.capabilities.can_read };
  creationKey.value = crypto.randomUUID();
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
    if (!editing.value) {
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
    result.value = null;
    recipient.value = '';
    status.value = '';
    linkMode.value = 'read';
    linkExpiry.value = new Date(Date.now() + 7 * 86400000);
    linkKey.value = crypto.randomUUID();
    history.value = false;
    deliveryKeys.clear();
    void load();
  },
  { immediate: true },
);
onBeforeUnmount(() => {
  version += 1;
});
watch(
  [email, rights, expiry],
  () => {
    creationKey.value = crypto.randomUUID();
  },
  { deep: true },
);
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
        : 'Invitation email queued, not yet delivered.';
  } catch (e) {
    status.value = `Invitation saved. ${failure(e, 'Email could not be queued; retry this invitation.')}`;
  }
}
async function savePerson() {
  if (!view.value || busy.value) {
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
    if (editing.value) {
      await axios.patch(`${base.value}/grants/${editing.value.id}`, {
        ...settings,
        version: editing.value.version,
        user_id: editing.value.user_id,
      });
      status.value = 'Direct access updated. Inherited access is unchanged.';
    } else {
      const response = await axios.post<AccessInvitation>(
        `${base.value}/invitations`,
        { email: email.value, ...settings },
        { headers: { 'Idempotency-Key': creationKey.value } },
      );
      await sendInvitation(response.data);
    }
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
  }
}
function edit(grant: GrantAccess) {
  editing.value = grant;
  email.value = grant.recipient_email;
  expiry.value = grant.expires_at ? new Date(grant.expires_at) : null;
  rights.value = {
    can_read: grant.can_read,
    can_upload: grant.can_upload,
    can_delete: grant.can_delete,
    can_share: grant.can_share,
  };
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
    error.value = failure(e, 'The link could not be created. Retry uses the same request.');
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
    status.value = 'Clipboard unavailable. Select and copy the link address.';
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
      response.data.status === 'sent'
        ? 'Link email sent.'
        : 'Link email queued, not yet delivered.';
  } catch (e) {
    error.value = failure(e, 'Could not queue this email. Retry sends this same link.');
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
    status.value = 'Access removed. Other inherited or independently granted access may remain.';
    await load();
  } catch (e) {
    error.value = failure(e, 'Access could not be removed. Refresh before retrying.');
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="sharing-panel">
    <p class="scope-label">
      Scope:
      {{
        scope.kind === 'client'
          ? 'entire client workspace'
          : `${scope.itemType === 'folder' ? 'folder and its descendants' : 'this file only'}`
      }}
      — {{ scope.name }}
    </p>
    <p v-if="loading" role="status">Loading access…</p>
    <div v-if="error" role="alert">
      <p>{{ error }}</p>
      <Button label="Refresh access" severity="secondary" :disabled="busy" @click="load" />
    </div>
    <p v-if="status" role="status" class="delivery-status">{{ status }}</p>
    <template v-if="view && !loading">
      <p v-if="scope.kind === 'item'" class="muted">
        Client and parent-folder access is inherited. A direct share adds permissions; it cannot
        restrict existing access.
      </p>
      <p v-if="view.inherits_client_access || view.inherits_folder_access" class="muted">
        Your access includes
        {{ view.inherits_client_access ? 'inherited client access' : 'a parent-folder share' }}.
        This dialog manages direct access to this item only, not inherited recipients.
      </p>
      <section aria-label="People with access">
        <h3>People</h3>
        <p v-if="view.people && !view.can_manage" class="muted">
          You can manage your own invitees, not independent members or staff.
        </p>
        <div v-for="person in view.people" :key="person.user_id" class="access-row">
          <div>
            <strong>{{ person.name || person.email }}</strong
            ><small>{{ person.email }} · {{ describeCapabilities(person.capabilities) }}</small>
          </div>
          <Button
            v-if="person.can_remove"
            label="Remove"
            severity="secondary"
            :disabled="busy"
            @click="confirmRemove({ kind: 'member', item: person })"
          />
        </div>
        <div v-for="grant in visibleGrants" :key="grant.id" class="access-row">
          <div>
            <strong>{{ grant.recipient_name || grant.recipient_email }}</strong
            ><small
              >Direct access · {{ describeCapabilities(grant) }} ·
              {{ grant.revoked_at ? 'Removed' : date(grant.expires_at) }}</small
            >
          </div>
          <div class="row-buttons">
            <Button
              v-if="canEdit(grant)"
              label="Edit"
              severity="secondary"
              :disabled="busy"
              @click="edit(grant)"
            /><Button
              v-if="active(grant)"
              label="Remove"
              severity="secondary"
              :disabled="busy"
              @click="confirmRemove({ kind: 'grant', item: grant })"
            />
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
              · {{ date(invitation.expires_at) }}</small
            >
          </div>
          <div v-if="active(invitation)" class="row-buttons">
            <Button
              label="Email invitation"
              severity="secondary"
              :disabled="busy"
              @click="sendInvitation(invitation)"
            /><Button
              label="Revoke"
              severity="secondary"
              :disabled="busy"
              @click="confirmRemove({ kind: 'invitation', item: invitation })"
            />
          </div>
        </div>
        <form class="access-form" @submit.prevent="savePerson">
          <h4>{{ editing ? 'Edit direct access' : 'Invite by email' }}</h4>
          <label :for="`${key}-email`">Recipient email</label
          ><InputText
            :id="`${key}-email`"
            v-model="email"
            type="email"
            autocomplete="email"
            required
            :disabled="busy || !!editing"
            fluid
          />
          <fieldset :disabled="busy">
            <legend>Permissions — limited to your current access</legend>
            <label
              ><Checkbox
                v-model="rights.can_read"
                binary
                :disabled="!view.capabilities.can_read"
                :inputId="`${key}-read`"
              />
              View & download</label
            >
            <label v-if="!isFile"
              ><Checkbox
                v-model="rights.can_upload"
                binary
                :disabled="!view.capabilities.can_upload"
                :inputId="`${key}-upload`"
              />
              Upload</label
            >
            <label v-if="view.capabilities.can_delete"
              ><Checkbox v-model="rights.can_delete" binary :inputId="`${key}-delete`" />
              Delete</label
            >
            <label
              ><Checkbox
                v-model="rights.can_share"
                binary
                :disabled="!view.capabilities.can_share"
                :inputId="`${key}-reshare`"
              />
              Allow resharing</label
            >
          </fieldset>
          <template v-if="scope.kind === 'item'"
            ><label :for="`${key}-expiry`">Access expiry (optional) · {{ timezone }}</label
            ><DatePicker
              v-model="expiry"
              :inputId="`${key}-expiry`"
              showTime
              hourFormat="24"
              showClear
              :minDate="new Date()"
              :disabled="busy"
              fluid
          /></template>
          <div class="row-buttons">
            <Button
              type="submit"
              :label="editing ? 'Save direct access' : 'Invite'"
              :loading="busy"
              :disabled="!Object.values(rights).some(Boolean)"
            /><Button
              v-if="editing"
              label="Cancel edit"
              severity="secondary"
              :disabled="busy"
              @click="resetForm"
            />
          </div>
          <p class="muted">
            Invitations are identity-bound. Account setup credentials are sent only by email, never
            copied to the inviter.
          </p>
        </form>
      </section>
      <section aria-label="Bearer links">
        <h3>Links</h3>
        <p class="muted">
          Anyone holding an active link can use its permitted actions. Removing a person does not
          invalidate known links.
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
            ><small>{{ link.revoked_at ? 'Revoked' : date(link.expires_at) }}</small>
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
          <label :for="`${key}-link-expiry`">Expires · {{ timezone }}</label
          ><DatePicker
            v-model="linkExpiry"
            :inputId="`${key}-link-expiry`"
            showTime
            hourFormat="24"
            :minDate="new Date()"
            :disabled="busy"
            required
            fluid
          />
          <p class="muted">
            Target: {{ scope.name }} · Anyone with the link · {{ date(linkExpiry?.toISOString()) }}
          </p>
          <Button
            type="submit"
            label="Create link"
            :loading="busy"
            :disabled="!linkMode || !linkExpiry"
          />
        </form>
        <div v-if="result && active(result)" class="access-form" aria-label="Created link">
          <h4 ref="resultHeading" tabindex="-1">Link ready — {{ scope.name }}</h4>
          <p class="muted">
            Copying does not send email. Recipient view will ask before switching your current
            portal session.
          </p>
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
            <label :for="`${key}-link-email`">Email this existing link</label
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
      </section>
      <label class="history-toggle"
        ><Checkbox v-model="history" binary :inputId="`${key}-history`" /> Show expired, accepted
        and revoked history</label
      >
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
        This removes the client membership, all direct grants and pending invitations for this
        person in this client. Their onward shares and known bearer links survive. Independent staff
        or inherited authority may remain.
      </p>
      <p v-else-if="removal?.kind === 'link'">
        This revokes this bearer link, including already-open recipient sessions. Other grants and
        links are unchanged.
      </p>
      <p v-else>
        This removes only this direct grant or pending invitation. Inherited access and independent
        links may still permit access.
      </p>
      <label v-if="removal?.kind === 'member'" class="history-toggle"
        ><Checkbox v-model="revokeLinks" binary :inputId="`${key}-revoke-links`" /> Also revoke all
        client, file and folder links within this client</label
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
  gap: 16px;
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
  padding: 12px;
  background: var(--p-content-hover-background);
  border-radius: 6px;
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
  border-top: 1px solid var(--p-content-border-color);
  padding-top: 16px;
}
.access-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 0;
  border-bottom: 1px solid var(--p-content-border-color);
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
fieldset,
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
fieldset {
  gap: 12px 16px;
}
fieldset legend {
  margin-bottom: 8px;
  color: var(--p-text-muted-color);
  font-size: 12px;
}
fieldset label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 44px;
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
</style>
