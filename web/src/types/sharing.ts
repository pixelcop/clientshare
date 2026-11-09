import type { AccessCapabilities, AccessInvitation, ItemGrant, SecureLink } from './models';

export type SharingScope =
  | { kind: 'client'; id: string; name: string }
  | { kind: 'item'; id: string; name: string; itemType: string };
export interface GrantAccess extends ItemGrant {
  recipient_name: string;
  recipient_email: string;
}
export interface MemberAccess {
  user_id: string;
  name: string;
  email: string;
  role: string;
  capabilities: AccessCapabilities;
  can_remove: boolean;
}
export interface SharingAccess {
  capabilities: AccessCapabilities;
  inherits_client_access?: boolean;
  inherits_folder_access?: boolean;
  can_manage?: boolean;
  grants?: GrantAccess[];
  people?: MemberAccess[];
  invitations: AccessInvitation[];
  links?: SecureLink[];
}
export function describeCapabilities(c: AccessCapabilities) {
  return [
    c.can_read && 'View & download',
    c.can_upload && 'Upload',
    c.can_delete && 'Delete',
    c.can_share && 'Share with others',
  ]
    .filter(Boolean)
    .join(' · ');
}
