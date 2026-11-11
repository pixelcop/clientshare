import { flushPromises, mount } from '@vue/test-utils';
import axios from 'axios';
import Button from 'openvue/button';
import OpenVue from 'openvue/config';
import Select from 'openvue/select';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import ScopedGroupAccess from '@/components/ScopedGroupAccess.vue';
import type { GroupDetails, TenantAccessRole } from '@/types/accessRoles';
import type { User } from '@/types/models';
import type { AdditionalAccess } from '@/types/sharing';

vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: { role: 'admin' } }) }));
const features = vi.hoisted(() => ({ groups: true }));
vi.mock('@/features', () => ({
  get GROUPS_UI_ENABLED() {
    return features.groups;
  },
}));

const people = [
  { id: 'internal', role: 'manager', access_profile: { user_type: 'internal' } },
  // The explicit identity takes priority over the compatibility role.
  { id: 'external', role: 'manager', access_profile: { user_type: 'external' } },
  { id: 'legacy-external', role: 'customer' },
] as User[];
const group = (id: string, member_ids: string[]) =>
  ({ id, name: id, member_ids, assignments: [], version: 1 }) as GroupDetails;
const groups = [
  group('internal-team', ['internal']),
  group('external-team', ['external', 'legacy-external']),
  group('mixed-team', ['internal', 'external']),
  group('unknown-team', ['missing']),
];
const roles = [
  { id: 'internal-role', name: 'Team manager', audience: 'internal' },
  { id: 'external-role', name: 'Client reviewer', audience: 'external' },
  { id: 'shared-role', name: 'Viewer', audience: 'both' },
  { id: 'archived-role', name: 'Old viewer', audience: 'both', archived_at: '2026-01-01' },
  { id: 'file-role', name: 'File reviewer', audience: 'both', allow_workspace: false },
].map((role) => ({
  allow_workspace: true,
  allow_folder: true,
  allow_file: true,
  can_read: true,
  can_upload: false,
  can_delete: false,
  can_invite_external: false,
  can_create_links: false,
  can_manage_access: false,
  ...role,
})) as TenantAccessRole[];

async function open() {
  const wrapper = mount(ScopedGroupAccess, {
    props: {
      scope: { kind: 'client', id: 'workspace', name: 'Workspace' },
      clientId: 'workspace',
      roles,
      assignments: [],
    },
    global: { plugins: [OpenVue], components: { Select, Button } },
  });
  await flushPromises();
  return wrapper;
}
type Wrapper = Awaited<ReturnType<typeof open>>;
function groupSelect(wrapper: Wrapper) {
  return wrapper.findAllComponents(Select).find((s) => s.props('ariaLabel') === 'Group')!;
}
function roleSelect(wrapper: Wrapper) {
  return wrapper
    .findAllComponents(Select)
    .find((s) => s.props('ariaLabel') === 'Role at this scope')!;
}
function options(wrapper: Wrapper) {
  return roleSelect(wrapper)
    .props('options')
    .map((r: TenantAccessRole) => r.id);
}

describe('scoped group role eligibility', () => {
  beforeEach(() => {
    features.groups = true;
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })),
    );
    vi.spyOn(axios, 'get').mockImplementation(async (url) => ({
      data: url === '/api/users' ? people : groups,
    }));
  });
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('hides group controls and assignments without hiding additional person access', async () => {
    features.groups = false;
    const wrapper = await open();
    expect(wrapper.find('form').exists()).toBe(false);
    expect(axios.get).not.toHaveBeenCalled();
    await wrapper.setProps({
      assignments: [
        { id: 'group', group_id: 'team', group_name: 'Hidden team', role_id: 'shared-role' },
        {
          id: 'person',
          user_id: 'external',
          recipient_name: 'Visible person',
          role_id: 'shared-role',
        },
      ] as AdditionalAccess[],
    });
    expect(wrapper.text()).not.toContain('Hidden team');
    expect(wrapper.text()).toContain('Visible person');
    expect(wrapper.find('button[aria-label="Remove group from this scope"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it.each([
    ['external-team', ['external-role', 'shared-role']],
    ['mixed-team', ['shared-role']],
    ['internal-team', ['internal-role', 'shared-role']],
    ['unknown-team', []],
  ])('offers only roles eligible for every member of %s', async (id, expected) => {
    const wrapper = await open();
    expect(options(wrapper)).toEqual([]);
    groupSelect(wrapper).vm.$emit('update:modelValue', id);
    await flushPromises();
    expect(options(wrapper)).toEqual(expected);
    wrapper.unmount();
  });

  it.each(['external-team', 'mixed-team'])(
    'clears an internal-only selection when switching to %s',
    async (id) => {
      const wrapper = await open();
      groupSelect(wrapper).vm.$emit('update:modelValue', 'internal-team');
      await flushPromises();
      roleSelect(wrapper).vm.$emit('update:modelValue', 'internal-role');
      await flushPromises();
      expect(roleSelect(wrapper).props('modelValue')).toBe('internal-role');
      groupSelect(wrapper).vm.$emit('update:modelValue', id);
      await flushPromises();
      expect(roleSelect(wrapper).props('modelValue')).toBe('');
      expect((wrapper.get('button[type="submit"]').element as HTMLButtonElement).disabled).toBe(
        true,
      );
      wrapper.unmount();
    },
  );

  it('keeps a compatible role selected across group changes', async () => {
    const wrapper = await open();
    groupSelect(wrapper).vm.$emit('update:modelValue', 'internal-team');
    await flushPromises();
    roleSelect(wrapper).vm.$emit('update:modelValue', 'shared-role');
    await flushPromises();
    groupSelect(wrapper).vm.$emit('update:modelValue', 'mixed-team');
    await flushPromises();
    expect(roleSelect(wrapper).props('modelValue')).toBe('shared-role');
    wrapper.unmount();
  });

  it('refreshes membership before review and rejects newly incompatible selections', async () => {
    const wrapper = await open();
    groupSelect(wrapper).vm.$emit('update:modelValue', 'internal-team');
    await flushPromises();
    roleSelect(wrapper).vm.$emit('update:modelValue', 'internal-role');
    await flushPromises();
    vi.mocked(axios.get).mockImplementation(async (url) => ({
      data: url === '/api/users' ? people : [group('internal-team', ['internal', 'external'])],
    }));
    const post = vi.spyOn(axios, 'post');
    await wrapper.get('form').trigger('submit');
    await flushPromises();
    expect(post).not.toHaveBeenCalled();
    expect(roleSelect(wrapper).props('modelValue')).toBe('');
    expect(options(wrapper)).toEqual(['shared-role']);
    expect(wrapper.get('[role="alert"]').text()).toContain('every member');
    wrapper.unmount();
  });
});
