import { describe, expect, it } from 'vitest';

import { sharingRoleLabel, sharingRoles, withinCeiling } from '@/types/sharingRoles';

describe('sharing role presets', () => {
  it.each([false, true])('offers meaningful permission bundles for file=%s', (isFile) => {
    sharingRoles(isFile).forEach((role) => {
      const rights = role.capabilities;
      expect(rights.can_delete || rights.can_share ? rights.can_read : true).toBe(true);
      expect(isFile ? !rights.can_upload : true).toBe(true);
      expect(sharingRoleLabel(rights, isFile)).toBe(role.label);
    });
    expect(
      sharingRoles(isFile).find((role) => role.value === 'manager')?.capabilities.can_share,
    ).toBe(true);
  });

  it('never exceeds any of the sixteen actor capability ceilings', () => {
    Array.from({ length: 16 }, (_, mask) => mask).forEach((mask) => {
      const ceiling = {
        can_read: mask % 2 === 1,
        can_upload: Math.floor(mask / 2) % 2 === 1,
        can_delete: Math.floor(mask / 4) % 2 === 1,
        can_share: Math.floor(mask / 8) % 2 === 1,
      };
      [true, false].forEach((isFile) => {
        sharingRoles(isFile).forEach((role) => {
          const expected =
            (!role.capabilities.can_read || ceiling.can_read) &&
            (!role.capabilities.can_upload || ceiling.can_upload) &&
            (!role.capabilities.can_delete || ceiling.can_delete) &&
            (!role.capabilities.can_share || ceiling.can_share);
          expect(withinCeiling(role.capabilities, ceiling)).toBe(expected);
        });
      });
    });
  });

  it('recognizes custom access without rounding up to a stronger role', () => {
    expect(
      sharingRoleLabel(
        { can_read: true, can_upload: false, can_delete: false, can_share: true },
        false,
      ),
    ).toBe('Custom access');
    expect(sharingRoles(false).find((role) => role.value === 'uploader')?.capabilities).toEqual({
      can_read: false,
      can_upload: true,
      can_delete: false,
      can_share: false,
    });
  });
});
