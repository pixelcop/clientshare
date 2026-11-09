import type { SecureLink } from './models';

export type LinkAccessMode = 'read' | 'write' | 'readwrite';
export type LinkEmailStatus = 'queued' | 'sent' | 'failed' | 'unavailable';
export interface LinkEmailResult {
  status: LinkEmailStatus;
  link_id?: string;
  email_queued?: boolean;
}

export const linkAccessOptions = [
  { label: 'View & download', value: 'read' },
  { label: 'Upload only', value: 'write' },
  { label: 'View, download & upload', value: 'readwrite' },
];

export function linkAccessLabel(mode: string) {
  if (mode === 'view') {
    return 'View & download';
  }
  return (
    linkAccessOptions.find((option) => option.value === mode)?.label || 'Unsupported permissions'
  );
}

export function linkAccessDescription(mode: string) {
  if (mode === 'read' || mode === 'view') {
    return 'Recipients can view and download files. Uploads are disabled.';
  }
  if (mode === 'write') {
    return 'Recipients can upload files, but cannot see or download existing files.';
  }
  if (mode === 'readwrite') {
    return 'Recipients can view, download and upload files. They cannot edit or delete files.';
  }
  return 'This link cannot be used.';
}

export function isUsableLink(link: SecureLink, now = Date.now()) {
  return (
    !link.revoked_at &&
    Number.isFinite(Date.parse(link.expires_at)) &&
    Date.parse(link.expires_at) > now &&
    ['read', 'view', 'write', 'readwrite'].includes(link.access_type)
  );
}

export function formatLinkExpiry(value: string) {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) {
    return 'Unavailable';
  }
  return `${new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'long' }).format(date)} (${Intl.DateTimeFormat().resolvedOptions().timeZone})`;
}
