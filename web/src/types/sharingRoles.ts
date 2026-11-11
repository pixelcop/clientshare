import type { AccessCapabilities } from './models';

export interface SharingRole {
  value: string;
  label: string;
  description: string;
  capabilities: AccessCapabilities;
}

// These are sharing presets, not portal/account roles. Authorization stays server-side.
export function sharingRoles(isFile: boolean): SharingRole[] {
  const capabilities = (read: boolean, upload = false, remove = false, share = false) => ({
    can_read: read,
    can_upload: !isFile && upload,
    can_delete: remove,
    can_share: share,
  });
  return [
    {
      value: 'viewer',
      label: 'Viewer',
      description: 'View and download files',
      capabilities: capabilities(true),
    },
    ...(!isFile
      ? [
          {
            value: 'uploader',
            label: 'Uploader',
            description: 'Upload and view your own files and folders',
            capabilities: capabilities(false, true),
          },
          {
            value: 'contributor',
            label: 'Contributor',
            description: 'View, download and upload files',
            capabilities: capabilities(true, true),
          },
        ]
      : []),
    {
      value: 'editor',
      label: 'Editor',
      description: isFile
        ? 'View, download and delete this file'
        : 'View, download, upload and delete files',
      capabilities: capabilities(true, true, true),
    },
    {
      value: 'manager',
      label: 'Manager',
      description: isFile
        ? 'View, download, delete and share this file'
        : 'View, download, upload, delete and share files',
      capabilities: capabilities(true, true, true, true),
    },
  ];
}

export function sameCapabilities(a: AccessCapabilities, b: AccessCapabilities) {
  return withinCeiling(a, b) && withinCeiling(b, a);
}

export function withinCeiling(rights: AccessCapabilities, ceiling: AccessCapabilities) {
  const sharing = (c: AccessCapabilities) =>
    c.sharing || {
      invite_external: c.can_share,
      create_links: c.can_share,
      manage_access: c.can_share,
    };
  const a = sharing(rights);
  const b = sharing(ceiling);
  return (
    (!rights.can_read || ceiling.can_read) &&
    (!rights.can_upload || ceiling.can_upload) &&
    (!rights.can_delete || ceiling.can_delete) &&
    (!a.invite_external || b.invite_external) &&
    (!a.create_links || b.create_links) &&
    (!a.manage_access || b.manage_access)
  );
}

export function sharingRoleLabel(rights: AccessCapabilities, isFile: boolean) {
  return (
    sharingRoles(isFile).find((role) => sameCapabilities(role.capabilities, rights))?.label ||
    'Custom access'
  );
}
