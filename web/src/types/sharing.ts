import type {
  AccessCapabilities,
  AccessInvitation,
  ItemGrant,
  RoleAssignment,
  SecureLink,
} from './models';

export interface AdditionalAccess extends RoleAssignment {
  recipient_name: string;
  recipient_email: string;
  recipient_user_type: string;
  group_name: string;
  can_manage: boolean;
}

export type SharingScope =
  | { kind: 'client'; id: string; name: string }
  | { kind: 'item'; id: string; name: string; itemType: string };
// Go embeds these fields directly in the JSON response.
export interface GrantAccess extends Omit<ItemGrant, 'AccessCapabilities'>, AccessCapabilities {
  recipient_name: string;
  recipient_email: string;
  recipient_user_type?: string;
  can_manage?: boolean;
}
export interface MemberAccess {
  user_id: string;
  name: string;
  email: string;
  role: string;
  role_id?: string;
  user_type?: string;
  capabilities: AccessCapabilities;
  can_remove: boolean;
}
export interface SharingAccess {
  client_id?: string;
  additional_access?: AdditionalAccess[];
  capabilities: AccessCapabilities;
  inherits_client_access?: boolean;
  inherits_folder_access?: boolean;
  can_manage?: boolean;
  grants?: GrantAccess[];
  people?: MemberAccess[];
  invitations: (AccessInvitation & AccessCapabilities)[];
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
