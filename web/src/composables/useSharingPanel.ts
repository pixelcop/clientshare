// Sharing mutations are sequential so partial saves remain retryable without replaying successes.
/* eslint-disable no-await-in-loop, no-restricted-syntax */
import { useIntervalFn, useMediaQuery, useTimestamp } from '@vueuse/core';
import axios from 'axios';
import { computed, nextTick, onBeforeUnmount, ref, toRef, watch } from 'vue';

import type { TenantAccessRole } from '@/types/accessRoles';
import {
  applicableRole,
  defaultAccessRole,
  roleCapabilities,
  roleSummary,
  tenantRoleLabel,
} from '@/types/accessRoles';
import type { AccessCapabilities, AccessInvitation, SecureLink } from '@/types/models';
import type { GrantAccess, MemberAccess, SharingAccess, SharingScope } from '@/types/sharing';
import { withinCeiling } from '@/types/sharingRoles';

interface DraftPerson {
  id: string;
  requestKey: string;
  email: string;
  role: string;
  expiry: Date | null;
}
type EditableGrant = Pick<
  GrantAccess,
  | 'id'
  | 'role_id'
  | 'user_id'
  | 'version'
  | 'expires_at'
  | 'revoked_at'
  | 'recipient_name'
  | 'recipient_email'
  | 'recipient_user_type'
  | 'can_manage'
> &
  AccessCapabilities & { assignment?: boolean };
type Removal =
  | { kind: 'grant'; item: EditableGrant }
  | { kind: 'invitation'; item: AccessInvitation }
  | { kind: 'member'; item: MemberAccess }
  | { kind: 'link'; item: SecureLink };
type MobilePerson = Exclude<Removal, { kind: 'link' }>;

// One session per panel: switching child panels must preserve drafts, edits and retry keys.
export function useSharingPanel(getScope: () => SharingScope, close: () => void) {
  const scope = toRef(getScope);
  const now = useTimestamp({ scheduler: (update) => useIntervalFn(update, 30_000) });
  const isFile = computed(() => scope.value.kind === 'item' && scope.value.itemType !== 'folder');
  const roleTarget = computed(() => {
    if (isFile.value) {
      return 'file';
    }
    return scope.value.kind === 'client' ? 'workspace' : 'folder';
  });
  const catalog = ref<TenantAccessRole[]>([]);
  const base = computed(
    () => `/api/${scope.value.kind === 'item' ? 'files' : 'clients'}/${scope.value.id}`,
  );
  const key = computed(() => `${scope.value.kind}-${scope.value.id}`);
  const view = ref<SharingAccess | null>(null);
  const links = ref<SecureLink[]>([]);
  const loading = ref(false);
  const busy = ref(false);
  const error = ref('');
  const status = ref('');
  const history = ref(false);
  const activeTab = ref('people');
  const linkTab = ref('create');
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
  const email = ref('');
  const drafts = ref<DraftPerson[]>([]);
  const scopeSummary = computed(() => {
    if (scope.value.kind === 'client') {
      return 'Entire client workspace';
    }
    return scope.value.itemType === 'folder'
      ? 'This folder and everything inside'
      : 'This file only';
  });
  const composing = ref(false);
  const isMobile = useMediaQuery('(max-width: 639px)');
  const mobileScreen = ref<'share' | 'manage'>('share');
  const mobilePerson = ref<MobilePerson | null>(null);
  const mobileTitle = computed(() => {
    if (activeTab.value === 'links') {
      return 'Sharing links';
    }
    return mobileScreen.value === 'manage' ? 'Manage access' : 'Share';
  });
  function mobileBack() {
    if (activeTab.value === 'links') {
      activeTab.value = 'people';
    }
    mobileScreen.value = 'share';
  }
  function openMobileManage() {
    document.getElementById(`${key.value}-email`)?.blur();
    mobileScreen.value = 'manage';
  }
  watch(isMobile, (mobile) => {
    if (!mobile) {
      mobilePerson.value = null;
    }
  });
  function focusRecipientEntry() {
    const input = document.getElementById(`${key.value}-email`);
    if (isMobile.value) {
      input?.blur();
    } else {
      input?.focus();
    }
  }
  const deliveryKeys = new Map<string, string>();
  const linkMode = ref('read');
  const linkExpiry = ref<Date | null>(new Date(Date.now() + 7 * 86400000));
  const linkKey = ref(crypto.randomUUID());
  const result = ref<SecureLink | null>(null);
  const recipient = ref('');
  const memberEdits = ref<Record<string, string>>({});
  const grantEdits = ref<Record<string, { role: string; expiry: Date | null }>>({});
  const memberRevokeLinks = ref<Record<string, boolean>>({});
  const removeRole = '__remove__';
  const removeOption = {
    id: removeRole,
    name: 'Remove access',
    description: 'Remove access added here. Other access is unchanged.',
    destructive: true,
  };
  const removal = ref<Removal | null>(null);
  const revokeLinks = ref(false);
  function confirmRemove(value: Removal) {
    revokeLinks.value = false;
    removal.value = value;
  }
  function active(item: { revoked_at?: string; expires_at?: string; accepted_at?: string }) {
    return (
      !item.revoked_at &&
      !item.accepted_at &&
      (!item.expires_at || Date.parse(item.expires_at) > now.value)
    );
  }
  const visibleGrants = computed<EditableGrant[]>(() =>
    [
      ...(view.value?.grants || []),
      ...(view.value?.additional_access || [])
        .filter((a) => !a.group_id && a.user_id)
        .map((a) => ({
          ...a,
          user_id: a.user_id!,
          assignment: true,
          ...(catalog.value.find((r) => r.id === a.role_id)
            ? roleCapabilities(
                catalog.value.find((r) => r.id === a.role_id)!,
                isFile.value,
              )
            : blank()),
        })),
    ].filter(active),
  );
  const changedMembers = computed(() =>
    (view.value?.people || []).filter(
      (p) => memberEdits.value[p.user_id] && memberEdits.value[p.user_id] !== p.role_id,
    ),
  );
  const changedGrants = computed(() =>
    visibleGrants.value.filter((g) => {
      const edit = grantEdits.value[g.id];
      return (
        edit &&
        (edit.role !== g.role_id ||
          (edit.expiry?.getTime() ?? null) !== (g.expires_at ? Date.parse(g.expires_at) : null))
      );
    }),
  );
  const pendingChanges = computed(
    () => changedMembers.value.length + changedGrants.value.length > 0,
  );
  const dirty = computed(
    () =>
      pendingChanges.value ||
      (!view.value &&
        (Object.keys(memberEdits.value).length > 0 || Object.keys(grantEdits.value).length > 0)) ||
      drafts.value.length > 0 ||
      !!email.value.trim(),
  );
  const discardAction = ref<(() => void) | null>(null);
  function requestClose() {
    if (busy.value || removal.value || discardAction.value || mobilePerson.value) {
      return;
    }
    if (dirty.value) {
      discardAction.value = close;
    } else {
      close();
    }
  }
  function memberRoles(person: MemberAccess) {
    return catalog.value.filter(
      (r) =>
        (applicableRole(r, person.user_type || 'external', 'workspace') ||
          r.id === person.role_id) &&
        withinCeiling(roleCapabilities(r), view.value?.capabilities || blank()),
    );
  }
  function grantRoles(grant: EditableGrant) {
    return catalog.value.filter(
      (r) =>
        (applicableRole(r, grant.recipient_user_type || 'external', roleTarget.value) ||
          r.id === grant.role_id) &&
        withinCeiling(roleCapabilities(r, isFile.value), view.value?.capabilities || blank()),
    );
  }
  function grantEdit(grant: EditableGrant) {
    return (
      grantEdits.value[grant.id] || {
        role: grant.role_id || '',
        expiry: grant.expires_at ? new Date(grant.expires_at) : null,
      }
    );
  }
  function setGrantRole(grant: EditableGrant, role: string) {
    if (role === removeRole) {
      confirmRemove({ kind: 'grant', item: grant });
    } else {
      grantEdits.value[grant.id] = { ...grantEdit(grant), role };
    }
  }
  function setMemberRole(person: MemberAccess, role: string) {
    if (role === removeRole) {
      confirmRemove({ kind: 'member', item: person });
    } else {
      memberEdits.value[person.user_id] = role;
    }
  }
  const linkEmailKey = ref(crypto.randomUUID());
  const copiedLinkId = ref<string | null>(null);
  const copied = computed(() => !!result.value && copiedLinkId.value === result.value.id);
  const resultHeading = ref<HTMLElement | null>(null);
  let version = 0;
  let disposed = false;
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
  const roleOptions = computed(() =>
    catalog.value
      .filter(
        (r) =>
          applicableRole(r, 'external', roleTarget.value) &&
          withinCeiling(roleCapabilities(r, isFile.value), view.value?.capabilities || blank()),
      )
      .map((r) => ({
        value: r.id,
        label: `${r.name}${r.archived_at ? ' (archived)' : ''}`,
        description: roleSummary(r, isFile.value),
        capabilities: roleCapabilities(r, isFile.value),
      })),
  );
  const selectedRole = computed({
    get: () => selectedRoleId.value,
    set: (value: string | undefined) => {
      const role = roleOptions.value.find((r) => r.value === value);
      if (role) {
        selectedRoleId.value = role.value;
      }
    },
  });
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
      .map((invite) => {
        let state = invite.revoked_at ? 'Invitation revoked' : 'Invitation expired';
        if (invite.accepted_at) {
          state = 'Invitation accepted';
        }
        return { id: invite.id, email: invite.email, state };
      }),
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
  const visibleInvitations = computed(() => (view.value?.invitations || []).filter(active));
  const hasLinkHistory = computed(() => links.value.some((link) => !active(link)));
  const visibleLinks = computed(() =>
    links.value
      .filter((link) => history.value || active(link))
      .sort((left, right) => {
        const activityOrder = Number(!active(left)) - Number(!active(right));
        const creationOrder =
          (Date.parse(right.created_at) || 0) - (Date.parse(left.created_at) || 0);
        return activityOrder || creationOrder || right.id.localeCompare(left.id);
      }),
  );
  const relativeTime = new Intl.RelativeTimeFormat('en', { numeric: 'always' });
  function linkExpiryText(value?: string) {
    if (!value) {
      return 'No expiry';
    }
    const remaining = Date.parse(value) - now.value;
    const distance = Math.abs(remaining);
    if (distance < 60_000) {
      return remaining > 0 ? 'Expires in less than a minute' : 'Expired less than a minute ago';
    }
    let unit: Intl.RelativeTimeFormatUnit = 'day';
    let duration = 86_400_000;
    if (distance < 3_600_000) {
      unit = 'minute';
      duration = 60_000;
    } else if (distance < 86_400_000) {
      unit = 'hour';
      duration = 3_600_000;
    }
    const amount = Math.round(distance / duration) * Math.sign(remaining);
    return `${remaining > 0 ? 'Expires' : 'Expired'} ${relativeTime.format(amount, unit)}`;
  }
  const previewPeople = computed(() => {
    const people = [
      ...(view.value?.people || []).map((p) => ({ name: p.name || p.email, email: p.email })),
      ...visibleGrants.value.map((p) => ({
        name: p.recipient_name || p.recipient_email,
        email: p.recipient_email,
      })),
      ...visibleInvitations.value.map((p) => ({ name: p.email, email: p.email })),
    ];
    return [...new Map(people.map((p) => [p.email.toLowerCase(), p])).values()];
  });
  const mobilePersonDetails = computed(() => {
    const selected = mobilePerson.value;
    if (!selected) {
      return null;
    }
    if (selected.kind === 'member') {
      return {
        name: selected.item.name || selected.item.email,
        email: selected.item.email,
        role: memberEdits.value[selected.item.user_id] || selected.item.role_id,
        roles: memberRoles(selected.item),
      };
    }
    if (selected.kind === 'grant') {
      return {
        name: selected.item.recipient_name || selected.item.recipient_email,
        email: selected.item.recipient_email,
        role: grantEdit(selected.item).role,
        roles: grantRoles(selected.item),
      };
    }
    return {
      name: selected.item.email,
      email: selected.item.email,
      role: selected.item.role_id,
      roles: [],
    };
  });
  function mobileRoleLabel(id?: string) {
    return id === removeRole ? 'Access will be removed' : roleLabel(id);
  }
  function chooseMobileRole(role: string) {
    const selected = mobilePerson.value;
    if (busy.value || !selected) {
      return;
    }
    if (selected.kind === 'member') {
      setMemberRole(selected.item, role);
    }
    if (selected.kind === 'grant') {
      setGrantRole(selected.item, role);
    }
    mobilePerson.value = null;
  }
  function removeMobilePerson() {
    const selected = mobilePerson.value;
    mobilePerson.value = null;
    if (selected) {
      confirmRemove(selected);
    }
  }
  function setMobileExpiry(expiry: Date | null) {
    const selected = mobilePerson.value;
    if (selected?.kind === 'grant') {
      grantEdits.value[selected.item.id] = { ...grantEdit(selected.item), expiry };
    }
  }
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
  function failure(e: unknown, fallback: string) {
    return axios.isAxiosError(e) && typeof e.response?.data?.error === 'string'
      ? e.response.data.error
      : fallback;
  }
  function resetForm() {
    composing.value = false;
    email.value = '';
    drafts.value = [];
    selectedRoleId.value = defaultAccessRole(
      catalog.value.filter((r) => roleOptions.value.some((o) => o.value === r.id)),
    );
  }
  function discardChanges() {
    const action = discardAction.value;
    discardAction.value = null;
    memberEdits.value = {};
    grantEdits.value = {};
    memberRevokeLinks.value = {};
    resetForm();
    action?.();
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
      role: selectedRole.value,
      expiry: null,
    });
    email.value = '';
    error.value = '';
    status.value = '';
    composing.value = true;
    await nextTick();
    focusRecipientEntry();
  }
  function removeDraft(id: string) {
    drafts.value = drafts.value.filter((person) => person.id !== id);
    composing.value = drafts.value.length > 0;
    void nextTick(focusRecipientEntry);
  }
  function updateDraft(person: DraftPerson) {
    Object.assign(person, { requestKey: crypto.randomUUID() });
  }
  async function backToPeople() {
    resetForm();
    await nextTick();
    focusRecipientEntry();
  }
  async function load() {
    version += 1;
    const current = version;
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
      if (!drafts.value.length && !email.value.trim()) {
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
      busy.value = false;
      resetForm();
      memberEdits.value = {};
      grantEdits.value = {};
      memberRevokeLinks.value = {};
      discardAction.value = null;
      mobileScreen.value = 'share';
      mobilePerson.value = null;
      result.value = null;
      recipient.value = '';
      status.value = '';
      linkMode.value = 'read';
      linkExpiry.value = new Date(Date.now() + 7 * 86400000);
      linkKey.value = crypto.randomUUID();
      history.value = false;
      activeTab.value = 'people';
      linkTab.value = 'create';
      copiedLinkId.value = null;
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
  async function emailMobileInvitation() {
    const selected = mobilePerson.value;
    if (busy.value || selected?.kind !== 'invitation') {
      return;
    }
    busy.value = true;
    try {
      await sendInvitation(selected.item);
      mobilePerson.value = null;
    } finally {
      busy.value = false;
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
          (r) => r.id === person.role && applicableRole(r, 'external', roleTarget.value),
        );
        const role = tenantRole
          ? { capabilities: roleCapabilities(tenantRole, isFile.value) }
          : null;
        if (!role || !view.value || !withinCeiling(role.capabilities, view.value.capabilities)) {
          throw new Error('Invalid role');
        }
        const response = await axios.post<AccessInvitation>(
          `${endpoint}/invitations`,
          {
            email: person.email,
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
        focusRecipientEntry();
      }
    }
  }
  function canEdit(grant: EditableGrant) {
    const ceiling = view.value?.capabilities;
    return ceiling && grant.can_manage !== false && withinCeiling(grant, ceiling);
  }
  async function saveChanges() {
    if (!view.value || busy.value || !pendingChanges.value) {
      return;
    }
    const target = key.value;
    busy.value = true;
    error.value = '';
    status.value = '';
    try {
      for (const person of [...changedMembers.value]) {
        if (disposed || target !== key.value) {
          return;
        }
        const role = memberEdits.value[person.user_id];
        if (role === removeRole) {
          await axios.delete(`${base.value}/members/${person.user_id}`, {
            data: { revoke_links: memberRevokeLinks.value[person.user_id] || false },
          });
          if (disposed || target !== key.value) {
            return;
          }
          view.value.people = view.value.people?.filter((p) => p.user_id !== person.user_id);
        } else {
          if (!memberRoles(person).some((r) => r.id === role)) {
            throw new Error('Choose an available workspace role.');
          }
          await axios.put(`${base.value}/members/${person.user_id}/role`, {
            role_id: role,
            expected_role_id: person.role_id || '',
          });
          if (disposed || target !== key.value) {
            return;
          }
          person.role_id = role;
        }
        delete memberEdits.value[person.user_id];
      }
      for (const grant of [...changedGrants.value]) {
        if (disposed || target !== key.value) {
          return;
        }
        const edit = grantEdits.value[grant.id];
        const endpoint = grant.assignment
          ? `/api/role-assignments/${grant.id}`
          : `${base.value}/grants/${grant.id}`;
        if (edit.role === removeRole) {
          await axios.delete(endpoint, { data: { version: grant.version } });
          if (disposed || target !== key.value) {
            return;
          }
          view.value.grants = view.value.grants?.filter((g) => g.id !== grant.id);
          view.value.additional_access = view.value.additional_access?.filter(
            (g) => g.id !== grant.id,
          );
        } else {
          const role = grantRoles(grant).find((r) => r.id === edit.role);
          if (!role || !canEdit(grant)) {
            throw new Error('Choose an available access role.');
          }
          // Changing only expiry must not round an existing permission bundle up or down.
          const capabilities =
            edit.role === grant.role_id
              ? {
                  can_read: grant.can_read,
                  can_upload: grant.can_upload,
                  can_delete: grant.can_delete,
                  can_share: grant.can_share,
                  ...(grant.sharing ? { sharing: grant.sharing } : {}),
                }
              : roleCapabilities(role, isFile.value);
          const response = await axios.patch(endpoint, {
            role_id: edit.role,
            expires_at:
              (edit.expiry?.getTime() ?? null) ===
              (grant.expires_at ? Date.parse(grant.expires_at) : null)
                ? grant.expires_at || null
                : edit.expiry?.toISOString() || null,
            version: grant.version,
            ...(!grant.assignment ? { capabilities, user_id: grant.user_id } : {}),
          });
          if (disposed || target !== key.value) {
            return;
          }
          const source = grant.assignment
            ? view.value.additional_access?.find((a) => a.id === grant.id)
            : grant;
          if (source) {
            Object.assign(source, capabilities, {
              role_id: edit.role,
              expires_at: edit.expiry?.toISOString() || undefined,
              version: response.data?.version ?? grant.version + 1,
            });
          }
        }
        delete grantEdits.value[grant.id];
      }
      if (disposed || target !== key.value) {
        return;
      }
      status.value = 'Access updated. Access through the client or another folder is unchanged.';
      await load();
      if (!error.value && !dirty.value) {
        close();
      }
    } catch (e) {
      if (target === key.value) {
        error.value = failure(
          e,
          'Some changes could not be saved. Saved changes are kept; retry the remaining changes.',
        );
      }
    } finally {
      busy.value = false;
    }
  }
  async function createLink() {
    if (busy.value || !linkExpiry.value) {
      return;
    }
    const target = key.value;
    busy.value = true;
    error.value = '';
    status.value = '';
    try {
      const response = await axios.post<SecureLink>(
        `${base.value}/links`,
        {
          access_type: linkMode.value,
          expiry_days: 7,
          expires_at: linkExpiry.value.toISOString(),
        },
        { headers: { 'Idempotency-Key': linkKey.value } },
      );
      if (disposed || target !== key.value) {
        return;
      }
      result.value = response.data;
      links.value = [response.data, ...links.value.filter((link) => link.id !== response.data.id)];
      recipient.value = '';
      copiedLinkId.value = null;
      await nextTick();
      resultHeading.value?.focus();
    } catch (e) {
      if (!disposed && target === key.value) {
        error.value = failure(e, 'The link could not be created. Please try again.');
      }
    } finally {
      if (!disposed && target === key.value) {
        busy.value = false;
      }
    }
  }
  function createAnotherLink() {
    result.value = null;
    linkKey.value = crypto.randomUUID();
  }
  async function copy(link: SecureLink) {
    try {
      await navigator.clipboard.writeText(link.link_url || '');
      copiedLinkId.value = link.id;
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
  async function remove() {
    const selected = removal.value;
    if (!selected) {
      return;
    }
    if (selected.kind === 'grant') {
      grantEdits.value[selected.item.id] = { ...grantEdit(selected.item), role: removeRole };
      removal.value = null;
      return;
    }
    if (selected.kind === 'member') {
      memberEdits.value[selected.item.user_id] = removeRole;
      memberRevokeLinks.value[selected.item.user_id] = revokeLinks.value;
      removal.value = null;
      return;
    }
    busy.value = true;
    error.value = '';
    try {
      if (selected.kind === 'invitation') {
        await axios.delete(`/api/invitations/${selected.item.id}`);
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

  return {
    scope,
    key,
    view,
    links,
    loading,
    busy,
    error,
    status,
    history,
    activeTab,
    linkTab,
    email,
    drafts,
    scopeSummary,
    composing,
    isMobile,
    mobileScreen,
    mobilePerson,
    mobileTitle,
    mobileBack,
    openMobileManage,
    memberEdits,
    grantEdits,
    removeRole,
    removeOption,
    pendingChanges,
    discardAction,
    requestClose,
    discardChanges,
    memberRoles,
    grantRoles,
    grantEdit,
    setGrantRole,
    setMemberRole,
    copied,
    copiedLinkId,
    resultHeading,
    isFile,
    catalog,
    roleLabel,
    canInvite,
    canCreateLinks,
    roleOptions,
    selectedRole,
    active,
    pastPeople,
    modes,
    visibleGrants,
    visibleInvitations,
    hasLinkHistory,
    visibleLinks,
    linkExpiryText,
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
    saveChanges,
    canEdit,
    linkMode,
    linkExpiry,
    result,
    recipient,
    createLink,
    createAnotherLink,
    copy,
    emailLink,
    removal,
    revokeLinks,
    confirmRemove,
    remove,
  };
}

export type SharingPanelSession = ReturnType<typeof useSharingPanel>;
