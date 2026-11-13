<script setup lang="ts">
import axios from 'axios';

import AccessSelect from '@/components/AccessSelect.vue';
import { GROUPS_UI_ENABLED } from '@/features';
import { replaySensitiveSelectPT } from '@/services/replayPrivacy';
import type { GroupDetails, TenantAccessRole } from '@/types/accessRoles';
import { roleSummary, tenantRoleLabel } from '@/types/accessRoles';
import type { Client, RolePermissions, User } from '@/types/models';

const roles = ref<TenantAccessRole[]>([]);
const groups = ref<GroupDetails[]>([]);
const users = ref<User[]>([]);
const clients = ref<Client[]>([]);
const loading = ref(false);
const busy = ref(false);
const error = ref('');
const status = ref('');
const tab = ref('roles');
const roleEditor = ref<TenantAccessRole | null>(null);
const groupEditor = ref<{
  id: string;
  name: string;
  description: string;
  version: number;
  member_ids: string[];
  assignments: { client_id: string; file_id?: string; scope_type?: string; role_id: string }[];
} | null>(null);
const permissionFields: { key: keyof RolePermissions; label: string }[] = [
  { key: 'can_read', label: 'View and download' },
  { key: 'can_upload', label: 'Upload into folders' },
  { key: 'can_delete', label: 'Delete documents' },
  { key: 'can_invite_external', label: 'Invite external people' },
  { key: 'can_create_links', label: 'Create sharing links' },
  { key: 'can_manage_access', label: 'Manage people’s access' },
];
const review = ref<{ message: string; action: () => Promise<unknown> } | null>(null);
const replacement = ref('');
const deleteRole = ref<TenantAccessRole | null>(null);
function message(e: unknown) {
  return axios.isAxiosError(e)
    ? e.response?.data?.error || 'Access settings could not be saved.'
    : 'Access settings could not be saved.';
}
async function load() {
  loading.value = true;
  error.value = '';
  try {
    const [r, g, u, c] = await Promise.all([
      axios.get<TenantAccessRole[]>('/api/access-roles?archived=true'),
      axios.get<GroupDetails[]>('/api/access-groups'),
      axios.get<User[]>('/api/users'),
      axios.get<Client[]>('/api/clients'),
    ]);
    roles.value = r.data;
    groups.value = g.data;
    users.value = u.data;
    clients.value = c.data;
  } catch (e) {
    error.value = message(e);
  } finally {
    loading.value = false;
  }
}
onMounted(load);
function editRole(role?: TenantAccessRole) {
  error.value = '';
  replacement.value = '';
  deleteRole.value = null;
  roleEditor.value = role
    ? { ...role }
    : {
        id: '',
        tenant_id: '',
        name: '',
        description: '',
        audience: 'both',
        allow_workspace: true,
        allow_folder: true,
        allow_file: true,
        can_read: true,
        can_upload: false,
        can_delete: false,
        can_invite_external: false,
        can_create_links: false,
        can_manage_access: false,
        legacy_sharing: false,
        version: 0,
        created_at: '',
        updated_at: '',
      };
}
async function reviewRole() {
  if (!roleEditor.value || busy.value) {
    return;
  }
  busy.value = true;
  error.value = '';
  try {
    const role = { ...roleEditor.value };
    const body = {
      name: role.name,
      description: role.description,
      audience: role.audience,
      allow_workspace: role.allow_workspace,
      allow_folder: role.allow_folder,
      allow_file: role.allow_file,
      ...Object.fromEntries(permissionFields.map((field) => [field.key, role[field.key]])),
    };
    if (!role.id) {
      await axios.post('/api/access-roles', body);
      roleEditor.value = null;
      status.value = 'Access role created.';
      await load();
      return;
    }
    const { data } = await axios.get<{
      memberships: number;
      grants: number;
      invitations: number;
      group_assignments: number;
      group_members: number;
      fingerprint: string;
      version: number;
    }>(`/api/access-roles/${role.id}/impact`);
    review.value = {
      message: `Update “${role.name}”? This changes ${data.memberships} workspace membership(s), ${data.grants} item grant(s), ${data.invitations} invitation(s), and ${data.group_assignments} group/additive assignment(s), including ${data.group_members} group member(s). Existing recipients receive permission changes immediately.`,
      action: () =>
        axios.put(`/api/access-roles/${role.id}`, {
          ...body,
          expected_version: data.version,
          impact: data.fingerprint,
        }),
    };
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = false;
  }
}
async function archive(role: TenantAccessRole, remove = false) {
  if (busy.value) {
    return;
  }
  busy.value = true;
  error.value = '';
  try {
    const { data } = await axios.get<{ fingerprint: string; version: number }>(
      `/api/access-roles/${role.id}/impact`,
    );
    const body = {
      expected_version: data.version,
      impact: data.fingerprint,
      replacement_id: replacement.value,
      replacement_version: roles.value.find((r) => r.id === replacement.value)?.version || 0,
    };
    review.value = {
      message: remove
        ? `Delete “${role.name}”? Existing assignments will use “${tenantRoleLabel(roles.value, replacement.value)}”. Review the replacement permissions before confirming.`
        : `Archive “${role.name}”? Existing recipients keep their access. This role will no longer be available for new assignments.`,
      action: () =>
        remove
          ? axios.delete(`/api/access-roles/${role.id}`, { data: body })
          : axios.post(`/api/access-roles/${role.id}/archive`, body),
    };
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = false;
  }
}
function editGroup(group?: GroupDetails) {
  error.value = '';
  groupEditor.value = group
    ? {
        id: group.id,
        name: group.name,
        description: group.description,
        version: group.version,
        member_ids: [...group.member_ids],
        assignments: group.assignments.map((a) => ({
          client_id: a.client_id,
          file_id: a.file_id,
          scope_type: a.scope_type,
          role_id: a.role_id,
        })),
      }
    : { id: '', name: '', description: '', version: 0, member_ids: [], assignments: [] };
}
function groupRoles(assignment: { file_id?: string; scope_type?: string; role_id: string }) {
  const scope = assignment.file_id ? assignment.scope_type : 'workspace';
  if (scope !== 'workspace' && scope !== 'folder' && scope !== 'file') {
    return [];
  }
  if (assignment.file_id && scope === 'workspace') {
    return [];
  }
  const kinds = (groupEditor.value?.member_ids || []).map((id) => {
    const u = users.value.find((u) => u.id === id);
    return (
      u?.access_profile?.user_type ||
      (['admin', 'manager'].includes(u?.role || '') ? 'internal' : 'external')
    );
  });
  return roles.value.filter(
    (r) =>
      (r.id === assignment.role_id || !r.archived_at) &&
      kinds.every((kind) => r.audience === 'both' || r.audience === kind) &&
      { workspace: r.allow_workspace, folder: r.allow_folder, file: r.allow_file }[scope],
  );
}
watch(
  [groupEditor, roles, users],
  () => {
    for (const assignment of groupEditor.value?.assignments || []) {
      if (
        assignment.role_id &&
        !groupRoles(assignment).some((role) => role.id === assignment.role_id)
      ) {
        assignment.role_id = '';
        review.value = null;
      }
    }
  },
  { deep: true },
);
async function reviewGroup(remove = false) {
  if (!groupEditor.value || busy.value) {
    return;
  }
  if (
    !remove &&
    groupEditor.value.assignments.some((a) => !groupRoles(a).some((role) => role.id === a.role_id))
  ) {
    error.value = 'Choose an eligible role for each assignment’s exact scope and group members.';
    return;
  }
  busy.value = true;
  error.value = '';
  try {
    const group = structuredClone(toRaw(groupEditor.value));
    if (remove) {
      group.member_ids = [];
      group.assignments = [];
    }
    const endpoint = `/api/access-groups${group.id ? `/${group.id}` : ''}`;
    const { data } = await axios.post<{
      members: number;
      assignments: number;
      previous_members: number;
      previous_assignments: number;
      fingerprint: string;
    }>(`${endpoint}/preview`, group);
    review.value = {
      message: `${remove ? 'Delete' : 'Save'} “${group.name}”? Members: ${data.previous_members} → ${data.members}. Scoped assignments: ${data.previous_assignments} → ${data.assignments}. Group members receive all listed roles; removing group access leaves independent grants unchanged.`,
      action: () =>
        remove
          ? axios.delete(endpoint, { data: { ...group, impact: data.fingerprint } })
          : group.id
            ? axios.put(endpoint, { ...group, impact: data.fingerprint })
            : axios.post(endpoint, { ...group, impact: data.fingerprint }),
    };
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = false;
  }
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
    roleEditor.value = null;
    groupEditor.value = null;
    deleteRole.value = null;
    status.value = 'Access settings saved.';
    await load();
  } catch (e) {
    error.value = message(e);
    review.value = null;
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <section class="grid gap-6" aria-label="Access roles">
    <header class="flex flex-wrap justify-between items-start gap-4">
      <div>
        <h2 class="text-lg font-semibold">Document access roles</h2>
        <p class="mt-2 text-muted-color max-w-2xl">
          Define document permissions once, then assign them to people in a client workspace,
          folder, or file. Organization administration stays separate.
        </p>
      </div>
      <Button v-if="tab === 'roles'" label="Create access role" @click="editRole()" />
    </header>
    <p v-if="error" role="alert" class="text-red-600">
      {{ error }} <Button label="Refresh" severity="secondary" @click="load" />
    </p>
    <p v-if="status" role="status">{{ status }}</p>
    <p v-if="loading" role="status">Loading access settings…</p>
    <nav v-if="GROUPS_UI_ENABLED" aria-label="Access settings" class="flex gap-2">
      <Button
        label="Access roles"
        :severity="tab === 'roles' ? 'primary' : 'secondary'"
        @click="tab = 'roles'"
      /><Button
        label="Groups"
        :severity="tab === 'groups' ? 'primary' : 'secondary'"
        @click="tab = 'groups'"
      />
    </nav>
    <section v-if="tab === 'roles'" class="grid gap-4">
      <p class="text-sm text-muted-color">
        Names are editable. Renaming a role never changes its permissions. Editing permissions
        updates all existing assignments.
      </p>
      <article
        v-for="role in roles"
        :key="role.id"
        class="py-4 border-b border-surface grid sm:grid-cols-[1fr_auto] gap-3"
      >
        <div>
          <h3 class="text-base font-semibold">
            {{ role.name }}
            <span v-if="role.archived_at" class="text-sm text-muted-color">(archived)</span>
          </h3>
          <p class="mt-1 text-sm">{{ roleSummary(role) }}</p>
          <p class="mt-1 text-sm text-muted-color">
            {{
              role.audience === 'both'
                ? 'Internal and External'
                : role.audience === 'internal'
                  ? 'Internal only'
                  : 'External only'
            }}
            ·
            {{
              [
                role.allow_workspace && 'Workspaces',
                role.allow_folder && 'Folders',
                role.allow_file && 'Files',
              ]
                .filter(Boolean)
                .join(', ')
            }}
          </p>
        </div>
        <div class="flex gap-2 items-start">
          <Button label="Edit" severity="secondary" @click="editRole(role)" /><Button
            v-if="!role.archived_at"
            label="Archive"
            severity="secondary"
            @click="archive(role)"
          /><Button
            label="Delete"
            severity="danger"
            variant="text"
            @click="
              deleteRole = role;
              replacement = '';
            "
          />
        </div>
      </article>
    </section>
    <section v-else-if="GROUPS_UI_ENABLED" class="grid gap-4">
      <div class="flex justify-between items-center">
        <h2 class="text-lg font-semibold">Groups</h2>
        <Button label="Create group" @click="editGroup()" />
      </div>
      <p class="text-sm text-muted-color">
        A group collects people. Access comes from the roles assigned to that group. Groups cannot
        contain other groups.
      </p>
      <p v-if="!groups.length && !loading">
        No groups yet. Create a group to share a workspace with a team.
      </p>
      <article
        v-for="group in groups"
        :key="group.id"
        class="py-4 border-b border-surface flex justify-between gap-4"
      >
        <div>
          <h3 data-sentry-mask class="text-base font-semibold">{{ group.name }}</h3>
          <p class="text-sm text-muted-color">
            {{ group.member_ids.length }} members · {{ group.assignments.length }} access
            assignments
          </p>
          <ul class="text-sm mt-2">
            <li v-for="a in group.assignments" :key="a.id">
              <span data-sentry-mask>{{ a.scope_name }}</span> · {{ tenantRoleLabel(roles, a.role_id) }}
            </li>
          </ul>
        </div>
        <Button label="Edit group" severity="secondary" @click="editGroup(group)" />
      </article>
    </section>
    <Dialog
      :visible="!!roleEditor"
      modal
      :dismissableMask="!busy"
      blockScroll
      header="Access role"
      class="w-[min(38rem,calc(100vw-2rem))]"
      @update:visible="
        (v) => {
          if (!v && !busy) roleEditor = null;
        }
      "
    >
      <form v-if="roleEditor" class="grid gap-4" @submit.prevent="reviewRole">
        <label class="grid gap-2"
          >Name<InputText v-model="roleEditor.name" required maxlength="100"
        /></label>
        <label class="grid gap-2"
          >Description<InputText v-model="roleEditor.description" maxlength="500"
        /></label>
        <label class="grid gap-2"
          >Available to<AccessSelect
            ariaLabel="Available to"
            v-model="roleEditor.audience"
            :options="[
              {
                label: 'Internal and External',
                value: 'both',
                description: 'Assignable to team members, clients and advisers.',
              },
              {
                label: 'Internal only',
                value: 'internal',
                description: 'Assignable only to your internal team.',
              },
              {
                label: 'External only',
                value: 'external',
                description: 'Assignable only to clients and advisers.',
              },
            ]"
            optionLabel="label"
            optionValue="value"
        /></label>
        <fieldset>
          <legend class="font-semibold mb-2">Applicable scopes</legend>
          <div class="flex gap-4 flex-wrap">
            <label class="flex gap-2 items-center min-h-11"
              ><Checkbox v-model="roleEditor.allow_workspace" binary />Workspace</label
            ><label class="flex gap-2 items-center min-h-11"
              ><Checkbox v-model="roleEditor.allow_folder" binary />Folder</label
            ><label class="flex gap-2 items-center min-h-11"
              ><Checkbox v-model="roleEditor.allow_file" binary />File</label
            >
          </div>
        </fieldset>
        <p class="text-sm text-muted-color">
          For roles already assigned, audience and scope changes require a replacement role. Upload
          applies to workspace/folder contents, never a single file.
        </p>
        <fieldset>
          <legend class="font-semibold mb-2">Permissions</legend>
          <label
            v-for="field in permissionFields"
            :key="field.key"
            class="flex gap-3 items-center min-h-11"
            ><Checkbox v-model="roleEditor[field.key]" binary />{{ field.label }}</label
          >
        </fieldset>
        <p v-if="roleEditor.legacy_sharing" class="text-sm text-muted-color">
          Legacy sharing restrictions are preserved unless you change a sharing permission. Review
          those changes carefully.
        </p>
        <p v-if="error" role="alert" class="text-red-600">{{ error }}</p>
        <Button
          type="submit"
          :label="roleEditor.id ? 'Review changes' : 'Create role'"
          :loading="busy"
        />
      </form>
    </Dialog>
    <Dialog
      v-if="GROUPS_UI_ENABLED"
      :visible="!!groupEditor"
      modal
      :dismissableMask="!busy"
      blockScroll
      header="Group"
      class="w-[min(44rem,calc(100vw-2rem))]"
      @update:visible="
        (v) => {
          if (!v && !busy) groupEditor = null;
        }
      "
    >
      <form v-if="groupEditor" class="grid gap-4" @submit.prevent="reviewGroup()">
        <label class="grid gap-2"
          >Name<InputText v-model="groupEditor.name" required maxlength="100"
        /></label>
        <label class="grid gap-2"
          >Description<InputText v-model="groupEditor.description" maxlength="500"
        /></label>
        <label class="grid gap-2"
          >Members<MultiSelect
            v-model="groupEditor.member_ids"
              :options="users"
              :pt="replaySensitiveSelectPT"
            :optionLabel="
              (u: User) => `${u.name || u.email} · ${u.access_profile?.user_type || u.role}`
            "
            optionValue="id"
            filter
            display="chip"
        /></label>
        <h3 class="text-base font-semibold">Access assignments</h3>
        <div
          v-for="(a, index) in groupEditor.assignments"
          :key="index"
          class="grid gap-2 border-b border-surface pb-4"
        >
          <label class="grid gap-2"
            >Client workspace<Select
              :ariaLabel="`Client workspace for assignment ${index + 1}`"
              v-model="a.client_id"
              :options="clients"
              :pt="replaySensitiveSelectPT"
              optionLabel="name"
              optionValue="id"
              filter
              required
              :disabled="!!a.file_id"
          /></label>
          <p v-if="a.file_id" data-sentry-mask class="text-sm text-muted-color">
            Scoped item:
            {{
              groups
                .find((g) => g.id === groupEditor?.id)
                ?.assignments.find((r) => r.file_id === a.file_id)?.scope_name || a.file_id
            }}. The scope is preserved.
          </p>
          <label class="grid gap-2"
            >Access role<AccessSelect
              :isFile="a.scope_type === 'file'"
              :ariaLabel="`Access role for assignment ${index + 1}`"
              v-model="a.role_id"
              :options="groupRoles(a)"
              optionLabel="name"
              optionValue="id"
              required
          /></label>
          <p class="text-sm text-muted-color">
            {{
              roles.find((r) => r.id === a.role_id)
                ? roleSummary(roles.find((r) => r.id === a.role_id)!)
                : 'Choose a role eligible for every member.'
            }}
          </p>
          <Button
            label="Remove assignment"
            severity="secondary"
            @click="groupEditor.assignments.splice(index, 1)"
          />
        </div>
        <Button
          label="Add workspace assignment"
          severity="secondary"
          @click="groupEditor.assignments.push({ client_id: '', role_id: '' })"
        />
        <p class="text-sm text-muted-color">
          Members receive all assignments immediately. Removing them from the group leaves their
          direct access unchanged.
        </p>
        <p v-if="error" role="alert" class="text-red-600">{{ error }}</p>
        <div class="flex justify-between gap-2">
          <Button
            v-if="groupEditor.id"
            label="Delete group"
            severity="danger"
            variant="text"
            :disabled="busy"
            @click="reviewGroup(true)"
          /><Button type="submit" label="Review group access" :loading="busy" />
        </div>
      </form>
    </Dialog>
    <Dialog
      :visible="!!deleteRole"
      modal
      :dismissableMask="!busy"
      header="Replace and delete role"
      class="w-[min(32rem,calc(100vw-2rem))]"
      @update:visible="
        (v) => {
          if (!v) deleteRole = null;
        }
      "
    >
      <div v-if="deleteRole" class="grid gap-4">
        <p>
          Choose the replacement for “{{ deleteRole.name }}”. A replacement is required when the
          role has assignments.
        </p>
        <label class="grid gap-2"
          >Replacement role<AccessSelect
            ariaLabel="Replacement role"
            v-model="replacement"
            :options="roles.filter((r) => r.id !== deleteRole?.id && !r.archived_at)"
            optionLabel="name"
            optionValue="id"
            showClear
        /></label>
        <p v-if="replacement" class="text-sm">
          {{ roleSummary(roles.find((r) => r.id === replacement)!) }}
        </p>
        <Button
          label="Review deletion"
          severity="danger"
          :loading="busy"
          @click="archive(deleteRole!, true)"
        />
      </div>
    </Dialog>
    <Dialog
      :visible="!!review"
      modal
      :dismissableMask="!busy"
      header="Review access change"
      class="w-[min(34rem,calc(100vw-2rem))]"
      :closable="!busy"
      @update:visible="
        (v) => {
          if (!v && !busy) review = null;
        }
      "
    >
      <div class="grid gap-5">
        <p data-sentry-mask>{{ review?.message }}</p>
        <div class="flex justify-end gap-2">
          <Button
            label="Back"
            severity="secondary"
            :disabled="busy"
            @click="review = null"
          /><Button label="Confirm change" :loading="busy" @click="confirm" />
        </div>
      </div>
    </Dialog>
  </section>
</template>
