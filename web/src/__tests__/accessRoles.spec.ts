import { describe, expect, it } from 'vitest';

import type { TenantAccessRole } from '@/types/accessRoles';
import {
  applicableRole,
  defaultAccessRole,
  roleCapabilities,
  roleSummary,
} from '@/types/accessRoles';
import { withinCeiling } from '@/types/sharingRoles';

const viewer = {
  id: 'tenant-viewer',
  name: 'Auditor',
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
} as TenantAccessRole;

describe('tenant access roles', () => {
  it('uses stable IDs and actual rights rather than editable role names', () => {
    const stronger = {
      ...viewer,
      id: 'stronger',
      name: 'Viewer',
      can_delete: true,
      template_key: 'viewer',
    };
    expect(defaultAccessRole([stronger, viewer])).toBe('tenant-viewer');
    expect(roleSummary(viewer)).toBe('View and download');
    expect(roleSummary(stronger)).toContain('Delete');
  });
  it('keeps scope and audience eligibility separate', () => {
    expect(applicableRole(viewer, 'external', 'file')).toBe(true);
    expect(applicableRole({ ...viewer, audience: 'internal' }, 'external', 'workspace')).toBe(
      false,
    );
    expect(applicableRole({ ...viewer, allow_file: false }, 'internal', 'file')).toBe(false);
    expect(
      applicableRole({ ...viewer, archived_at: new Date().toISOString() }, 'external', 'file'),
    ).toBe(false);
  });
  it('projects uploads away on a single file without dropping deletion', () => {
    const manager = { ...viewer, can_upload: true, can_delete: true };
    expect(roleCapabilities(manager, true)).toMatchObject({
      can_read: true,
      can_upload: false,
      can_delete: true,
    });
    expect(manager.can_upload).toBe(true);
  });
  it('never lets a link author or inviter grant independent sharing rights they lack', () => {
    const inviter = roleCapabilities({ ...viewer, can_invite_external: true });
    const links = roleCapabilities({ ...viewer, can_create_links: true });
    const manager = roleCapabilities({ ...viewer, can_manage_access: true });
    expect(withinCeiling(roleCapabilities(viewer), inviter)).toBe(true);
    expect(withinCeiling(links, inviter)).toBe(false);
    expect(withinCeiling(manager, links)).toBe(false);
    expect(
      withinCeiling(
        { can_read: true, can_upload: false, can_delete: false, can_share: true },
        inviter,
      ),
    ).toBe(false);
  });
});
