<script setup lang="ts">
import axios from 'axios';

import AccessSelect from '@/components/AccessSelect.vue';
import { GROUPS_UI_ENABLED } from '@/features';
import { replaySensitiveSelectPT } from '@/services/replayPrivacy';
import { useAuthStore } from '@/stores/auth';
import type { GroupDetails, TenantAccessRole } from '@/types/accessRoles';
import { applicableRole, roleSummary, tenantRoleLabel } from '@/types/accessRoles';
import type { User } from '@/types/models';
import type { AdditionalAccess, SharingScope } from '@/types/sharing';
const props = defineProps<{
  scope: SharingScope;
  clientId: string;
  roles: TenantAccessRole[];
  assignments: AdditionalAccess[];
}>();
const emit = defineEmits<{ changed: [] }>();
const auth = useAuthStore();
const groups = ref<GroupDetails[]>([]);
const users = ref<User[]>([]);
const groupId = ref('');
const roleId = ref('');
const busy = ref(false);
const error = ref('');
const review = ref<{ text: string; action: () => Promise<unknown> } | null>(null);
const edits = ref<Record<string, string>>({});
const kind = computed(() =>
  props.scope.kind === 'client'
    ? 'workspace'
    : props.scope.itemType === 'folder'
      ? 'folder'
      : 'file',
);
function rolesForGroup(group?: GroupDetails) {
  if (!group) {
    return [];
  }
  const memberTypes = group.member_ids.map((id) => {
    const user = users.value.find((person) => person.id === id);
    if (!user) {
      return undefined;
    }
    if (user.access_profile) {
      const type = user.access_profile.user_type;
      return type === 'internal' || type === 'external' ? type : undefined;
    }
    if (['admin', 'manager'].includes(user.role)) {
      return 'internal';
    }
    if (['client', 'customer'].includes(user.role)) {
      return 'external';
    }
    return undefined;
  });
  return props.roles.filter(
    (role) =>
      !role.archived_at &&
      { workspace: role.allow_workspace, folder: role.allow_folder, file: role.allow_file }[
        kind.value
      ] &&
      memberTypes.every((type) => !!type && applicableRole(role, type, kind.value)),
  );
}
const availableRoles = computed(() =>
  rolesForGroup(groups.value.find((g) => g.id === groupId.value)),
);
watch(groupId, () => {
  review.value = null;
});
watch(availableRoles, (roles) => {
  if (!roles.some((role) => role.id === roleId.value)) {
    roleId.value = '';
    review.value = null;
  }
});
onMounted(async () => {
  if (GROUPS_UI_ENABLED && auth.user?.role === 'admin') {
    try {
      const [groupList, people] = await Promise.all([
        axios.get<GroupDetails[]>('/api/access-groups'),
        axios.get<User[]>('/api/users'),
      ]);
      users.value = people.data;
      groups.value = groupList.data;
    } catch {
      error.value = 'Groups could not be loaded.';
    }
  }
});
async function stageGroup(removeId?: string) {
  if (busy.value) {
    return;
  }
  busy.value = true;
  error.value = '';
  try {
    const id = removeId
      ? props.assignments.find((a) => a.id === removeId)?.group_id
      : groupId.value;
    if (!id) {
      return;
    }
    const [groupList, people] = await Promise.all([
      axios.get<GroupDetails[]>('/api/access-groups'),
      axios.get<User[]>('/api/users'),
    ]);
    users.value = people.data;
    groups.value = groupList.data;
    const group = groupList.data.find((g) => g.id === id);
    if (!group) {
      throw new Error();
    }
    if (!removeId && !rolesForGroup(group).some((role) => role.id === roleId.value)) {
      roleId.value = '';
      review.value = null;
      error.value = 'Choose a role available to every member of this group.';
      return;
    }
    const existing = group.assignments.map((a) => ({
      client_id: a.client_id,
      file_id: a.file_id,
      role_id: a.role_id,
    }));
    const fileId = props.scope.kind === 'item' ? props.scope.id : undefined;
    const assignments = existing.filter(
      (a) => !(a.client_id === props.clientId && a.file_id === fileId),
    );
    if (!removeId) {
      assignments.push({ client_id: props.clientId, file_id: fileId, role_id: roleId.value });
    }
    const input = {
      name: group.name,
      description: group.description,
      member_ids: group.member_ids,
      assignments,
      version: group.version,
    };
    const { data } = await axios.post<{ fingerprint: string }>(
      `/api/access-groups/${id}/preview`,
      input,
    );
    review.value = {
      text: `${removeId ? 'Remove' : 'Assign'} access for “${group.name}” at ${props.scope.name}? This affects ${group.member_ids.length} member(s). Other access stays unchanged.`,
      action: () => axios.put(`/api/access-groups/${id}`, { ...input, impact: data.fingerprint }),
    };
  } catch (e) {
    error.value = axios.isAxiosError(e)
      ? e.response?.data?.error || 'Group access could not be reviewed.'
      : 'Group access could not be reviewed.';
  } finally {
    busy.value = false;
  }
}
function stagePerson(a: AdditionalAccess, revoke = false) {
  review.value = {
    text: `${revoke ? 'Remove this assignment' : 'Update this role'} for ${a.recipient_email}? Other access stays unchanged.`,
    action: () =>
      revoke
        ? axios.delete(`/api/role-assignments/${a.id}`, { data: { version: a.version } })
        : axios.patch(`/api/role-assignments/${a.id}`, {
            version: a.version,
            role_id: edits.value[a.id] || a.role_id,
            expires_at: a.expires_at || null,
          }),
  };
}
async function confirm() {
  if (!review.value || busy.value) {
    return;
  }
  busy.value = true;
  error.value = '';
  try {
    await review.value.action();
    review.value = null;
    emit('changed');
  } catch (e) {
    error.value = axios.isAxiosError(e)
      ? e.response?.data?.error || 'Access could not be updated.'
      : 'Access could not be updated.';
    review.value = null;
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <section
    v-if="
      assignments.some((a) => GROUPS_UI_ENABLED || !a.group_id) ||
      (GROUPS_UI_ENABLED && auth.user?.role === 'admin')
    "
    class="grid gap-3 mt-5"
    aria-label="Additional access"
  >
    <h3 class="font-semibold">Additional access</h3>
    <p class="text-sm text-muted-color">
      Each assignment is independent. Removing one leaves other access sources unchanged.
    </p>
    <article
      v-for="a in assignments.filter(
        (a) =>
          (GROUPS_UI_ENABLED || !a.group_id) &&
          !a.revoked_at &&
          (!a.expires_at || Date.parse(a.expires_at) > Date.now()),
      )"
      :key="a.id"
      class="grid gap-2 border-b border-surface pb-3"
    >
      <strong data-sentry-mask>{{ a.group_name || a.recipient_name || a.recipient_email }}</strong>
      <p class="text-sm text-muted-color">
        {{ a.group_id ? 'Group access' : 'Additional invitation assignment' }} ·
        {{ tenantRoleLabel(roles, a.role_id) }}
      </p>
      <div v-if="a.can_manage && a.user_id" class="flex gap-2 flex-wrap">
        <AccessSelect
          :isFile="kind === 'file'"
          v-model="edits[a.id]"
          :options="
            roles.filter(
              (r) => r.id === a.role_id || applicableRole(r, a.recipient_user_type, kind),
            )
          "
          optionLabel="name"
          optionValue="id"
          :ariaLabel="`Role for ${a.recipient_email}`"
          :placeholder="tenantRoleLabel(roles, a.role_id)"
        /><Button
          label="Save role"
          severity="secondary"
          :disabled="!edits[a.id] || busy"
          @click="stagePerson(a)"
        /><Button
          label="Remove assignment"
          severity="danger"
          variant="text"
          :disabled="busy"
          @click="stagePerson(a, true)"
        />
      </div>
      <Button
        v-if="a.group_id && auth.user?.role === 'admin'"
        label="Remove group from this scope"
        severity="secondary"
        :disabled="busy"
        @click="stageGroup(a.id)"
      />
    </article>
    <form
      v-if="GROUPS_UI_ENABLED && auth.user?.role === 'admin'"
      class="grid gap-3"
      @submit.prevent="stageGroup()"
    >
      <label class="grid gap-2"
        >Group<Select
          :pt="replaySensitiveSelectPT"
          ariaLabel="Group"
          v-model="groupId"
          :options="groups"
          optionLabel="name"
          optionValue="id"
          filter
          placeholder="Choose a group"
      /></label>
      <label class="grid gap-2"
        >Role at this scope<AccessSelect
          :isFile="kind === 'file'"
          ariaLabel="Role at this scope"
          v-model="roleId"
          :options="availableRoles"
          optionLabel="name"
          optionValue="id"
          placeholder="Choose an access role"
      /></label>
      <p v-if="roleId" class="text-sm text-muted-color">
        {{
          roleSummary(
            roles.find((r) => r.id === roleId)!,
            kind === 'file',
          )
        }}
      </p>
      <Button
        label="Review group access"
        type="submit"
        severity="secondary"
        :disabled="busy || !groupId || !roleId || !clientId"
      />
    </form>
    <p v-if="error" role="alert" class="text-red-600">{{ error }}</p>
    <div
      v-if="review"
      class="grid gap-3 border border-surface p-4"
      aria-label="Review scoped access"
    >
      <p data-sentry-mask>{{ review.text }}</p>
      <div class="flex gap-2">
        <Button
          label="Cancel"
          severity="secondary"
          :disabled="busy"
          @click="review = null"
        /><Button label="Confirm access change" :loading="busy" @click="confirm" />
      </div>
    </div>
  </section>
</template>
