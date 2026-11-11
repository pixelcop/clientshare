import type {
  AccessCapabilities,
  AccessGroup,
  AccessProfile,
  AccessRole,
  RoleAssignment,
  RolePermissions,
  UserClient,
} from './models';

export type TenantAccessRole = Omit<AccessRole, 'RolePermissions'> & RolePermissions;
export interface GroupDetails extends AccessGroup {
  member_ids: string[];
  assignments: RoleAssignment[];
}
export interface UserAccessSettings {
  profile: AccessProfile;
  workspaces: (Omit<UserClient, 'AccessCapabilities'> & AccessCapabilities)[];
  group_ids: string[];
}
export interface WorkspaceAccessInput {
  client_id: string;
  role_id: string;
}

export function roleCapabilities(role: TenantAccessRole, isFile = false): AccessCapabilities {
  return {
    can_read: role.can_read,
    can_upload: !isFile && role.can_upload,
    can_delete: role.can_delete,
    can_share: role.can_invite_external || role.can_create_links || role.can_manage_access,
    ...(role.legacy_sharing
      ? {}
      : {
          sharing: {
            invite_external: role.can_invite_external,
            create_links: role.can_create_links,
            manage_access: role.can_manage_access,
          },
        }),
  };
}

export function applicableRole(
  role: TenantAccessRole,
  userType: string,
  scope: 'workspace' | 'folder' | 'file',
) {
  return (
    !role.archived_at &&
    (role.audience === 'both' || role.audience === userType) &&
    { workspace: role.allow_workspace, folder: role.allow_folder, file: role.allow_file }[scope]
  );
}

export function roleSummary(role: TenantAccessRole, isFile = false) {
  return (
    [
      role.can_read && 'View and download',
      !isFile && role.can_upload && 'Upload',
      role.can_delete && 'Delete',
      role.can_invite_external && 'Invite external people',
      role.can_create_links && 'Create sharing links',
      role.can_manage_access && 'Manage access',
      role.legacy_sharing && 'Legacy sharing rules retained',
    ]
      .filter(Boolean)
      .join(' · ') || 'No document permissions'
  );
}

export function tenantRoleLabel(roles: TenantAccessRole[], id?: string) {
  const role = roles.find((item) => item.id === id);
  return role ? `${role.name}${role.archived_at ? ' (archived)' : ''}` : 'Legacy access';
}

// Choose by actual permissions, never by an editable name or template key.
export function defaultAccessRole(roles: TenantAccessRole[]) {
  return (
    roles.find(
      (r) =>
        !r.archived_at &&
        r.can_read &&
        !r.can_upload &&
        !r.can_delete &&
        !r.can_invite_external &&
        !r.can_create_links &&
        !r.can_manage_access,
    )?.id ||
    roles.find(
      (r) =>
        !r.archived_at &&
        !r.can_read &&
        r.can_upload &&
        !r.can_delete &&
        !r.can_invite_external &&
        !r.can_create_links &&
        !r.can_manage_access,
    )?.id ||
    ''
  );
}
