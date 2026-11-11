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
  return (Object.keys(a) as (keyof AccessCapabilities)[]).every((key) => a[key] === b[key]);
}

export function withinCeiling(rights: AccessCapabilities, ceiling: AccessCapabilities) {
  return (Object.keys(rights) as (keyof AccessCapabilities)[]).every(
    (key) => !rights[key] || ceiling[key],
  );
}

export function sharingRoleLabel(rights: AccessCapabilities, isFile: boolean) {
  return (
    sharingRoles(isFile).find((role) => sameCapabilities(role.capabilities, rights))?.label ||
    'Custom access'
  );
}
