import { flushPromises, mount } from '@vue/test-utils';
import axios from 'axios';
import OpenVue from 'openvue/config';
import Dialog from 'openvue/dialog';
import Select from 'openvue/select';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import SharingDialog from '@/components/SharingDialog.vue';
import SharingPanel from '@/components/SharingPanel.vue';

vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: { id: 'self' } }) }));

const capabilities = { can_read: true, can_upload: true, can_delete: true, can_share: true };
const roles = ['viewer', 'contributor'].map((id) => ({
  id,
  name: id,
  audience: 'both',
  allow_workspace: true,
  allow_folder: true,
  allow_file: true,
  can_read: true,
  can_upload: id === 'contributor',
  can_delete: false,
  can_invite_external: false,
  can_create_links: false,
  can_manage_access: false,
}));
const access = {
  capabilities,
  can_manage: true,
  invitations: [],
  people: [
    {
      user_id: 'self',
      name: 'Portal admin',
      email: 'admin@example.test',
      role: 'admin',
      can_remove: false,
      capabilities,
    },
    {
      user_id: 'member',
      name: 'Workspace member',
      email: 'member@example.test',
      role_id: 'viewer',
      can_remove: true,
      capabilities,
    },
  ],
  grants: [
    {
      id: 'grant',
      user_id: 'reader',
      role_id: 'viewer',
      version: 7,
      recipient_name: 'Direct reader',
      recipient_email: 'reader@example.test',
      can_manage: true,
      ...capabilities,
      can_upload: false,
      can_delete: false,
      can_share: false,
    },
  ],
  additional_access: [
    {
      id: 'additional',
      user_id: 'extra',
      role_id: 'viewer',
      version: 3,
      recipient_name: 'Additional reader',
      recipient_email: 'extra@example.test',
      can_manage: true,
    },
  ],
};

async function open() {
  const wrapper = mount(SharingDialog, {
    props: { visible: true, scope: { kind: 'client', id: 'workspace', name: 'Client workspace' } },
    global: { plugins: [OpenVue], stubs: { teleport: true, transition: false, RouterLink: true } },
    attachTo: document.body,
  });
  await flushPromises();
  return wrapper;
}
type Wrapper = Awaited<ReturnType<typeof open>>;
async function choose(wrapper: Wrapper, label: string, value: string) {
  wrapper
    .findAllComponents(Select)
    .find((s) => s.props('ariaLabel') === label)!
    .vm.$emit('update:modelValue', value);
  await flushPromises();
}

describe('staged people access edits', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })),
    );
    vi.spyOn(axios, 'get').mockImplementation(async (url) => {
      if (url === '/api/access-roles') {
        return { data: roles };
      }
      if (String(url).endsWith('/access')) {
        return { data: structuredClone(access) };
      }
      return { data: [] };
    });
  });
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('saves inline workspace, grant and additive roles together with their concurrency guards', async () => {
    const put = vi.spyOn(axios, 'put').mockResolvedValue({ data: {} });
    const patch = vi.spyOn(axios, 'patch').mockResolvedValue({ data: {} });
    const wrapper = await open();
    expect(wrapper.text()).toContain('Portal admin (you)');
    expect(wrapper.text()).toContain('Administrator');
    expect(wrapper.find('[aria-label="Save role"]').exists()).toBe(false);
    await choose(wrapper, 'Workspace role for member@example.test', 'contributor');
    await choose(wrapper, 'Role for reader@example.test', 'contributor');
    await choose(wrapper, 'Role for extra@example.test', 'contributor');
    expect(put).not.toHaveBeenCalled();
    expect(patch).not.toHaveBeenCalled();
    await wrapper.get('button[aria-label="Save"]').trigger('click');
    await flushPromises();
    expect(put).toHaveBeenCalledExactlyOnceWith('/api/clients/workspace/members/member/role', {
      role_id: 'contributor',
      expected_role_id: 'viewer',
    });
    expect(patch).toHaveBeenCalledWith(
      '/api/clients/workspace/grants/grant',
      expect.objectContaining({ version: 7, role_id: 'contributor' }),
    );
    expect(patch).toHaveBeenCalledWith('/api/role-assignments/additional', {
      version: 3,
      role_id: 'contributor',
      expires_at: null,
    });
    expect(wrapper.emitted('close')).toHaveLength(1);
    wrapper.unmount();
  });

  it('keeps unsaved edits when the discard dialog is dismissed and discards only explicitly', async () => {
    const wrapper = await open();
    await choose(wrapper, 'Workspace role for member@example.test', 'contributor');
    const outer = wrapper.findAllComponents(Dialog)[0];
    expect(outer.props('dismissableMask')).toBe(true);
    outer.vm.$emit('update:visible', false);
    await flushPromises();
    const discard = wrapper
      .findAllComponents(Dialog)
      .find((d) => d.props('header') === 'Discard unsaved changes?')!;
    expect(discard.props('visible')).toBe(true);
    expect(discard.props('dismissableMask')).toBe(true);
    discard.vm.$emit('update:visible', false);
    await flushPromises();
    expect(wrapper.emitted('close')).toBeUndefined();
    expect(wrapper.get('button[aria-label="Save"]').exists()).toBe(true);
    outer.vm.$emit('update:visible', false);
    await flushPromises();
    await wrapper.get('button[aria-label="Discard"]').trigger('click');
    expect(wrapper.emitted('close')).toHaveLength(1);
    wrapper.unmount();
  });

  it('keeps failed changes retryable without replaying already saved members', async () => {
    const put = vi.spyOn(axios, 'put').mockResolvedValue({ data: {} });
    const patch = vi
      .spyOn(axios, 'patch')
      .mockRejectedValueOnce(new Error('Network error'))
      .mockResolvedValue({ data: {} });
    const wrapper = await open();
    await choose(wrapper, 'Workspace role for member@example.test', 'contributor');
    await choose(wrapper, 'Role for reader@example.test', 'contributor');
    await wrapper.get('button[aria-label="Save"]').trigger('click');
    await flushPromises();
    expect(wrapper.emitted('close')).toBeUndefined();
    expect(wrapper.text()).toContain('retry the remaining changes');
    await wrapper.get('button[aria-label="Save"]').trigger('click');
    await flushPromises();
    expect(put).toHaveBeenCalledTimes(1);
    expect(patch).toHaveBeenCalledTimes(2);
    expect(wrapper.emitted('close')).toHaveLength(1);
    wrapper.unmount();
  });

  it('stages removal and allows restoring the role before saving', async () => {
    const remove = vi.spyOn(axios, 'delete').mockResolvedValue({ data: {} });
    const wrapper = await open();
    await choose(wrapper, 'Workspace role for member@example.test', '__remove__');
    await wrapper.get('button[aria-label="Remove access"]').trigger('click');
    await flushPromises();
    expect(remove).not.toHaveBeenCalled();
    expect(wrapper.find('.pending-removal').exists()).toBe(true);
    await choose(wrapper, 'Workspace role for member@example.test', 'viewer');
    expect(wrapper.get('button[aria-label="Done"]').exists()).toBe(true);
    await choose(wrapper, 'Role for reader@example.test', '__remove__');
    await wrapper.get('button[aria-label="Remove access"]').trigger('click');
    await flushPromises();
    await wrapper.get('button[aria-label="Save"]').trigger('click');
    await flushPromises();
    expect(remove).toHaveBeenCalledExactlyOnceWith('/api/clients/workspace/grants/grant', {
      data: { version: 7 },
    });
    wrapper.unmount();
  });

  it('also protects unsent recipient drafts when closing', async () => {
    const wrapper = await open();
    await wrapper.get('input[type="email"]').setValue('unsent@example.test');
    wrapper.findComponent(SharingPanel).vm.requestClose();
    await flushPromises();
    expect(wrapper.text()).toContain('Discard unsaved changes?');
    expect(wrapper.emitted('close')).toBeUndefined();
    wrapper.unmount();
  });

  it('treats an unchanged expiry as unchanged across timezone representations', async () => {
    const snapshot = structuredClone(access);
    const expiresAt = '2027-03-01T12:00:00-05:00';
    Object.assign(snapshot.grants[0]!, { expires_at: expiresAt });
    vi.mocked(axios.get).mockImplementation(async (url) => {
      if (url === '/api/access-roles') return { data: roles };
      return { data: String(url).endsWith('/access') ? snapshot : [] };
    });
    const patch = vi.spyOn(axios, 'patch').mockResolvedValue({ data: {} });
    const wrapper = await open();
    await choose(wrapper, 'Role for reader@example.test', 'viewer');
    expect(wrapper.get('button[aria-label="Done"]').exists()).toBe(true);
    await choose(wrapper, 'Role for reader@example.test', 'contributor');
    await wrapper.get('button[aria-label="Save"]').trigger('click');
    await flushPromises();
    expect(patch).toHaveBeenCalledWith(
      '/api/clients/workspace/grants/grant',
      expect.objectContaining({ expires_at: expiresAt }),
    );
    wrapper.unmount();
  });
});
