<script setup lang="ts">
import axios from 'axios';

import AccessSelect from '@/components/AccessSelect.vue';
import StaffSeatUsage from '@/components/StaffSeatUsage.vue';
import { GROUPS_UI_ENABLED } from '@/features';
import { useAuthStore } from '@/stores/auth';
import { useStaffQuotaStore } from '@/stores/staffQuota';
import type { UserCreatePayload } from '@/stores/users';
import type { GroupDetails, TenantAccessRole, UserAccessSettings } from '@/types/accessRoles';
import {
  applicableRole,
  defaultAccessRole,
  roleCapabilities,
  roleSummary,
  tenantRoleLabel,
} from '@/types/accessRoles';
import type { Client, User } from '@/types/models';
import { withinCeiling } from '@/types/sharingRoles';

const props = defineProps<{
  show: boolean;
  mode: 'add' | 'edit';
  user?: User | null;
  clients: Client[];
  defaultRole?: 'admin' | 'manager' | 'client';
  defaultClientIds?: string[];
  error?: string;
  loading?: boolean;
}>();
const emit = defineEmits<{ submit: [UserCreatePayload]; close: [] }>();
const auth = useAuthStore();
const quota = useStaffQuotaStore();
const isAdmin = computed(() => auth.user?.role === 'admin');
const isEdit = computed(() => props.mode === 'edit');
const fieldPrefix = useId();
const name = ref('');
const email = ref('');
const userType = ref<'internal' | 'external'>('internal');
const organizationRole = ref<'member' | 'administrator'>('member');
const allWorkspaces = ref(false);
const legacyUserManagement = ref(false);
const hadLegacyUserManagement = ref(false);
const roles = ref<TenantAccessRole[]>([]);
const groups = ref<GroupDetails[]>([]);
const groupIds = ref<string[]>([]);
const clientIds = ref<string[]>([]);
const workspaceRoles = ref<Record<string, string>>({});
const applyRole = ref('');
const localError = ref('');
const preparing = ref(false);
const ready = ref(false);
let requestVersion = 0;
const existingStaff = computed(
  () => isEdit.value && ['admin', 'manager'].includes(props.user?.role || ''),
);
const staffBlocked = computed(() => !existingStaff.value && !quota.canAddStaff);
const invitationBlocked = computed(() => userType.value === 'internal' && staffBlocked.value);
const disabled = computed(() => props.loading || preparing.value || !ready.value);
const eligibleRoles = computed(() =>
  roles.value.filter((r) => applicableRole(r, userType.value, 'workspace')),
);
const selectedGroups = computed(() => groups.value.filter((g) => groupIds.value.includes(g.id)));
const self = computed(() => isEdit.value && props.user?.id === auth.user?.id);
const assignableClients = computed(() =>
  isAdmin.value
    ? props.clients
    : props.clients.filter(
        (c) =>
          c.capabilities?.can_share &&
          (!c.capabilities.sharing ||
            c.capabilities.sharing.invite_external ||
            c.capabilities.sharing.manage_access),
      ),
);
const bulkRoles = computed(() =>
  eligibleRoles.value.filter((r) =>
    clientIds.value.every((id) => optionsFor(id).some((option) => option.id === r.id)),
  ),
);

function optionsFor(clientId: string) {
  const current = roles.value.find((r) => r.id === workspaceRoles.value[clientId]);
  const ceiling = props.clients.find((c) => c.id === clientId)?.capabilities;
  const eligible = isAdmin.value
    ? eligibleRoles.value
    : eligibleRoles.value.filter(
        (r) => ceiling?.can_share && withinCeiling(roleCapabilities(r), ceiling),
      );
  return current?.archived_at &&
    isAdmin.value &&
    (current.audience === 'both' || current.audience === userType.value)
    ? [...eligible, current]
    : eligible;
}
function initialRole(clientId?: string) {
  return defaultAccessRole(clientId ? optionsFor(clientId) : eligibleRoles.value);
}
watch(clientIds, (ids) => {
  for (const id of ids) {
    if (!workspaceRoles.value[id]) {
      workspaceRoles.value[id] = initialRole(id);
    }
  }
});
watch(userType, () => {
  allWorkspaces.value = false;
  for (const id of clientIds.value) {
    if (!optionsFor(id).some((r) => r.id === workspaceRoles.value[id])) {
      workspaceRoles.value[id] = initialRole(id);
    }
  }
});
async function prepare() {
  const version = ++requestVersion;
  preparing.value = true;
  ready.value = false;
  hadLegacyUserManagement.value = false;
  legacyUserManagement.value = false;
  localError.value = '';
  name.value = props.user?.name || '';
  email.value = props.user?.email || '';
  userType.value = (props.user?.access_profile?.user_type ||
    (['client', 'customer'].includes(
      props.user?.role || props.defaultRole || (isAdmin.value ? 'manager' : 'client'),
    )
      ? 'external'
      : 'internal')) as 'internal' | 'external';
  organizationRole.value =
    props.user?.role === 'admin' || props.defaultRole === 'admin' ? 'administrator' : 'member';
  allWorkspaces.value = props.user?.manager_full_access || false;
  clientIds.value = [...(props.user?.client_ids || props.defaultClientIds || [])];
  workspaceRoles.value = {};
  groupIds.value = [];
  try {
    const [catalog, groupList, settings] = await Promise.all([
      axios.get<TenantAccessRole[]>('/api/access-roles', { params: { archived: isAdmin.value } }),
      isAdmin.value && GROUPS_UI_ENABLED
        ? axios.get<GroupDetails[]>('/api/access-groups')
        : Promise.resolve({ data: [] as GroupDetails[] }),
      isEdit.value && props.user
        ? axios.get<UserAccessSettings>(`/api/users/${props.user.id}/access-settings`)
        : Promise.resolve(null),
    ]);
    if (version !== requestVersion || !props.show) {
      return;
    }
    roles.value = catalog.data;
    groups.value = groupList.data;
    if (settings) {
      userType.value = settings.data.profile.user_type as 'internal' | 'external';
      organizationRole.value =
        settings.data.profile.organization_role === 'administrator' ? 'administrator' : 'member';
      allWorkspaces.value = settings.data.profile.all_workspaces;
      hadLegacyUserManagement.value =
        settings.data.profile.organization_role === 'member' &&
        settings.data.profile.can_manage_users;
      legacyUserManagement.value = hadLegacyUserManagement.value;
      clientIds.value = settings.data.workspaces.map((w) => w.client_id);
      groupIds.value = settings.data.group_ids;
      const missing = settings.data.workspaces
        .map((w) => w.role_id)
        .filter((id): id is string => !!id && !roles.value.some((r) => r.id === id));
      const preserved = await Promise.all(
        [...new Set(missing)].map((id) => axios.get<TenantAccessRole>(`/api/access-roles/${id}`)),
      );
      if (version !== requestVersion || !props.show) {
        return;
      }
      roles.value.push(...preserved.map((r) => r.data));
      workspaceRoles.value = Object.fromEntries(
        settings.data.workspaces.map((w) => [w.client_id, w.role_id || '']),
      );
    } else {
      workspaceRoles.value = Object.fromEntries(clientIds.value.map((id) => [id, initialRole(id)]));
    }
    applyRole.value = defaultAccessRole(bulkRoles.value);
    ready.value = true;
  } catch (e) {
    if (version === requestVersion) {
      localError.value = axios.isAxiosError(e)
        ? e.response?.data?.error || 'Access settings could not be loaded. Close and try again.'
        : 'Access settings could not be loaded.';
    }
  } finally {
    if (version === requestVersion) {
      preparing.value = false;
    }
  }
}
watch(
  () => props.show,
  (show) => {
    if (show) {
      void prepare();
      void quota.refresh();
    } else {
      requestVersion++;
    }
  },
  { immediate: true },
);
onBeforeUnmount(() => {
  requestVersion++;
});
function refreshQuotaOnFocus() {
  if (props.show) {
    void quota.refresh();
  }
}
onMounted(() => window.addEventListener('focus', refreshQuotaOnFocus));
onBeforeUnmount(() => window.removeEventListener('focus', refreshQuotaOnFocus));
function onSubmit() {
  if (disabled.value || invitationBlocked.value) {
    return;
  }
  localError.value = '';
  if (!name.value.trim() || !email.value.trim()) {
    localError.value = 'Name and email are required.';
    return;
  }
  if (
    clientIds.value.some((id) => !optionsFor(id).some((r) => r.id === workspaceRoles.value[id]))
  ) {
    localError.value = 'Choose an available access role for each workspace.';
    return;
  }
  const payload: UserCreatePayload = {
    name: name.value.trim(),
    email: email.value.trim(),
    user_type: userType.value,
    organization_role: userType.value === 'internal' ? organizationRole.value : 'none',
    all_workspaces:
      userType.value === 'internal' &&
      (organizationRole.value === 'administrator' || allWorkspaces.value),
    workspace_access: clientIds.value.map((id) => ({
      client_id: id,
      role_id: workspaceRoles.value[id]!,
    })),
    ...(isAdmin.value && GROUPS_UI_ENABLED ? { group_ids: groupIds.value } : {}),
    ...(isAdmin.value && isEdit.value && hadLegacyUserManagement.value
      ? { can_manage_users: userType.value === 'internal' && legacyUserManagement.value }
      : {}),
  };
  emit('submit', payload);
}
</script>

<template>
  <Dialog
    :visible="show"
    modal
    :dismissableMask="!loading"
    closable
    blockScroll
    :draggable="false"
    :header="isEdit ? 'Edit person' : 'Add person'"
    class="w-[min(42rem,calc(100vw-2rem))]"
    @update:visible="
      (visible) => {
        if (!visible) emit('close');
      }
    "
  >
    <form class="grid gap-5" @submit.prevent="onSubmit">
      <p class="text-muted-color">
        Identity and organization administration are separate from document access.
      </p>
      <StaffSeatUsage v-if="isAdmin && !existingStaff" />
      <p v-if="preparing" role="status">Loading roles and access…</p>
      <div class="grid gap-4 sm:grid-cols-2">
        <div class="grid gap-2">
          <label :for="`${fieldPrefix}-name`">Name</label
          ><InputText
            :id="`${fieldPrefix}-name`"
            v-model="name"
            type="text"
            autocomplete="name"
            required
            :disabled="disabled"
            fluid
          />
        </div>
        <div class="grid gap-2">
          <label :for="`${fieldPrefix}-email`">Email</label
          ><InputText
            :id="`${fieldPrefix}-email`"
            v-model="email"
            type="email"
            autocomplete="email"
            required
            :disabled="disabled"
            fluid
          />
        </div>
        <div class="grid gap-2">
          <label :id="`${fieldPrefix}-type-label`" :for="`${fieldPrefix}-type`">User type</label>
          <AccessSelect
            :inputId="`${fieldPrefix}-type`"
            :ariaLabelledby="`${fieldPrefix}-type-label`"
            v-model="userType"
            :options="
              isAdmin
                ? [
                    {
                      label: 'Internal — your team',
                      value: 'internal',
                      description: 'A team identity. Document access is assigned separately.',
                    },
                    {
                      label: 'External — clients and advisers',
                      value: 'external',
                      description:
                        'A client or adviser identity. Document access is assigned separately.',
                    },
                  ]
                : [
                    {
                      label: 'External — clients and advisers',
                      value: 'external',
                      description:
                        'A client or adviser identity. Document access is assigned separately.',
                    },
                  ]
            "
            optionLabel="label"
            optionValue="value"
            :disabled="disabled || self"
            fluid
          />
        </div>
        <div v-if="userType === 'internal'" class="grid gap-2">
          <label :id="`${fieldPrefix}-organization-label`" :for="`${fieldPrefix}-organization`"
            >Organization role</label
          >
          <AccessSelect
            :inputId="`${fieldPrefix}-organization`"
            :ariaLabelledby="`${fieldPrefix}-organization-label`"
            v-model="organizationRole"
            :options="[
              {
                label: 'Member',
                value: 'member',
                description:
                  'No organization administration. Document access is assigned separately.',
              },
              {
                label: 'Administrator',
                value: 'administrator',
                description:
                  'Manage settings, people and roles, with access to all client workspaces.',
              },
            ]"
            optionLabel="label"
            optionValue="value"
            :disabled="disabled || self"
            fluid
          />
        </div>
      </div>
      <p
        v-if="userType === 'internal' && organizationRole === 'administrator'"
        class="text-muted-color"
      >
        Administrators manage organization settings, people and roles, and retain access to all
        client workspaces.
      </p>
      <label v-else-if="userType === 'internal' && isAdmin" class="flex gap-3 items-center min-h-11"
        ><Checkbox v-model="allWorkspaces" binary :disabled="disabled" />Full access to all current
        and future client workspaces</label
      >
      <div
        v-if="hadLegacyUserManagement && userType === 'internal' && organizationRole === 'member'"
        class="grid gap-2"
      >
        <label class="flex gap-3 items-center min-h-11"
          ><Checkbox v-model="legacyUserManagement" binary :disabled="disabled" />Legacy
          organization management</label
        >
        <p class="text-sm text-muted-color">
          Retained from the former Manager role: manage external users and create client workspaces.
          Turn this off for a standard Member.
        </p>
      </div>
      <fieldset class="grid gap-3">
        <legend class="font-semibold mb-3">Direct workspace access</legend>
        <label :for="`${fieldPrefix}-clients`">Assign client workspaces</label>
        <MultiSelect
          :inputId="`${fieldPrefix}-clients`"
          v-model="clientIds"
          :options="assignableClients"
          optionLabel="name"
          optionValue="id"
          filter
          display="chip"
          placeholder="Choose workspaces"
          :disabled="disabled"
          fluid
        />
        <div v-if="clientIds.length > 1" class="flex gap-2 items-center flex-wrap">
          <AccessSelect
            v-model="applyRole"
            ariaLabel="Role to apply to all selected workspaces"
            :options="bulkRoles"
            optionLabel="name"
            optionValue="id"
            :disabled="disabled"
            placeholder="Choose a role"
          />
          <Button
            type="button"
            label="Apply to selected workspaces"
            severity="secondary"
            :disabled="disabled || !applyRole"
            @click="
              () => {
                for (const id of clientIds) workspaceRoles[id] = applyRole;
              }
            "
          />
        </div>
        <div
          v-for="clientId in clientIds"
          :key="clientId"
          class="grid gap-2 border-b border-surface pb-3 sm:grid-cols-2 items-center"
        >
          <label :id="`${fieldPrefix}-${clientId}-label`" :for="`${fieldPrefix}-${clientId}`">{{
            clients.find((c) => c.id === clientId)?.name || clientId
          }}</label>
          <AccessSelect
            :inputId="`${fieldPrefix}-${clientId}`"
            :ariaLabelledby="`${fieldPrefix}-${clientId}-label`"
            v-model="workspaceRoles[clientId]"
            :options="optionsFor(clientId)"
            :optionLabel="(r: TenantAccessRole) => tenantRoleLabel(roles, r.id)"
            optionValue="id"
            :disabled="disabled"
            placeholder="Choose an access role"
            fluid
          />
          <p class="text-sm text-muted-color sm:col-span-2">
            {{
              roles.find((r) => r.id === workspaceRoles[clientId])
                ? roleSummary(roles.find((r) => r.id === workspaceRoles[clientId])!)
                : 'Select a role to replace this legacy permission combination.'
            }}
          </p>
        </div>
        <p v-if="!clientIds.length" class="text-sm text-muted-color">
          No direct document access. Access may still come from a group, an item share, or
          all-workspace authority.
        </p>
      </fieldset>
      <div v-if="isAdmin && GROUPS_UI_ENABLED" class="grid gap-2">
        <label :for="`${fieldPrefix}-groups`">Groups</label>
        <MultiSelect
          :inputId="`${fieldPrefix}-groups`"
          v-model="groupIds"
          :options="groups"
          optionLabel="name"
          optionValue="id"
          filter
          display="chip"
          :disabled="disabled"
          fluid
          placeholder="Optional group membership"
        />
        <p class="text-sm text-muted-color">
          Adding a person to a group gives them its existing access. Removing direct workspace
          access does not remove group access.
        </p>
        <ul v-if="selectedGroups.length" class="grid gap-3 text-sm">
          <li v-for="group in selectedGroups" :key="group.id">
            <strong>{{ group.name }}</strong>
            <ul>
              <li
                v-for="assignment in group.assignments"
                :key="assignment.id"
                class="mt-1 text-muted-color"
              >
                {{
                  assignment.scope_name ||
                  clients.find((c) => c.id === assignment.client_id)?.name ||
                  'Shared item'
                }}
                · {{ tenantRoleLabel(roles, assignment.role_id) }}
              </li>
              <li v-if="!group.assignments.length" class="text-muted-color">
                No document access assigned to this group yet.
              </li>
            </ul>
          </li>
        </ul>
        <p v-if="selectedGroups.length" class="text-sm text-muted-color">
          Group access is live: future changes to these groups and their roles apply to their
          members.
        </p>
      </div>
      <p v-if="invitationBlocked" role="alert" class="text-muted-color">
        No staff seats are available. Add seats before inviting an internal team member.
      </p>
      <p v-if="localError || error" role="alert" class="text-red-600">{{ localError || error }}</p>
      <div class="flex justify-end gap-2">
        <Button
          type="button"
          label="Cancel"
          severity="secondary"
          :disabled="loading"
          @click="emit('close')"
        /><Button
          type="submit"
          :label="isEdit ? 'Save changes' : 'Create and invite'"
          :loading="loading"
          :disabled="disabled || invitationBlocked"
        />
      </div>
    </form>
  </Dialog>
</template>
