import { flushPromises, mount } from '@vue/test-utils';
import axios from 'axios';
import Button from 'openvue/button';
import OpenVue from 'openvue/config';
import Select from 'openvue/select';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { GroupDetails, TenantAccessRole } from '@/types/accessRoles';
import AccessSettings from '@/views/AccessSettings.vue';

// Keep deferred group behavior covered while the production UI is hidden.
const features = vi.hoisted(() => ({ groups: true }));
vi.mock('@/features', () => ({
  get GROUPS_UI_ENABLED() {
    return features.groups;
  },
}));

const roles = [
  { id: 'file-role', name: 'File reviewer', allow_file: true, allow_folder: false },
  { id: 'folder-role', name: 'Folder reviewer', allow_file: false, allow_folder: true },
  { id: 'both-role', name: 'Item reviewer', allow_file: true, allow_folder: true },
  { id: 'workspace-role', name: 'Workspace reviewer', allow_file: false, allow_folder: false },
].map((r) => ({
  audience: 'both',
  allow_workspace: true,
  can_read: true,
  ...r,
})) as TenantAccessRole[];

async function open(scopeType?: string, roleId = 'both-role') {
  const group = {
    id: 'team',
    name: 'Team',
    member_ids: ['external'],
    version: 1,
    assignments: [
      {
        id: 'assignment',
        client_id: 'workspace',
        file_id: 'item',
        scope_type: scopeType,
        role_id: roleId,
      },
    ],
  } as GroupDetails;
  const responses: Record<string, unknown> = {
    '/api/access-groups': [group],
    '/api/users': [{ id: 'external', role: 'client', access_profile: { user_type: 'external' } }],
    '/api/clients': [{ id: 'workspace', name: 'Workspace' }],
    '/api/access-roles?archived=true': roles,
  };
  vi.spyOn(axios, 'get').mockImplementation(async (url) => ({ data: responses[String(url)] }));
  const wrapper = mount(AccessSettings, {
    global: {
      plugins: [OpenVue],
      components: { Select, Button },
      stubs: { Dialog: { props: ['visible'], template: '<div v-if="visible"><slot /></div>' } },
    },
  });
  await flushPromises();
  if (!features.groups) return wrapper;
  await wrapper.get('button[aria-label="Groups"]').trigger('click');
  await wrapper.get('button[aria-label="Edit group"]').trigger('click');
  await flushPromises();
  return wrapper;
}
type Wrapper = Awaited<ReturnType<typeof open>>;
function roleSelect(wrapper: Wrapper) {
  return wrapper
    .findAllComponents(Select)
    .find((s) => s.props('ariaLabel') === 'Access role for assignment 1')!;
}

describe('group editor exact scope roles', () => {
  beforeEach(() => {
    features.groups = true;
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })),
    );
  });
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('hides group management while keeping role management available for this release', async () => {
    features.groups = false;
    const wrapper = await open();
    expect(wrapper.text()).toContain('Document access roles');
    expect(wrapper.find('button[aria-label="Create access role"]').exists()).toBe(true);
    expect(wrapper.find('button[aria-label="Groups"]').exists()).toBe(false);
    expect(wrapper.find('button[aria-label="Create group"]').exists()).toBe(false);
    expect(wrapper.find('button[aria-label="Edit group"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it.each([
    ['file', ['file-role', 'both-role']],
    ['folder', ['folder-role', 'both-role']],
    [undefined, []],
  ])('filters the dropdown by actual target type %s', async (scope, expected) => {
    const wrapper = await open(scope);
    expect(
      roleSelect(wrapper)
        .props('options')
        .map((r: TenantAccessRole) => r.id),
    ).toEqual(expected);
    wrapper.unmount();
  });

  it.each([
    ['file', 'folder-role'],
    ['folder', 'file-role'],
    [undefined, 'both-role'],
  ])('clears incompatible existing role %s/%s and blocks review', async (scope, role) => {
    const wrapper = await open(scope, role);
    expect(roleSelect(wrapper).props('modelValue')).toBe('');
    const post = vi.spyOn(axios, 'post');
    await wrapper
      .get('button[aria-label="Review group access"]')
      .element.closest('form')!
      .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    await flushPromises();
    expect(post).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain('exact scope');
    wrapper.unmount();
  });
});
