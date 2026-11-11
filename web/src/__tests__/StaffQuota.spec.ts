import { flushPromises, mount } from '@vue/test-utils';
import axios from 'axios';
import Checkbox from 'openvue/checkbox';
import OpenVue from 'openvue/config';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import StaffSeatUsage from '../components/StaffSeatUsage.vue';
import UserModal from '../components/UserModal.vue';
import { useAuthStore } from '../stores/auth';
import { useBrandingStore } from '../stores/branding';
import { useStaffQuotaStore } from '../stores/staffQuota';
import type { User } from '../types/models';

describe('staff invitation quota', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    useAuthStore().user = { id: 'admin', role: 'admin' } as User;
    vi.spyOn(axios, 'get').mockResolvedValue({ data: {} });
  });
  afterEach(() => vi.restoreAllMocks());

  it.each([
    [1, 2, false],
    [2, 2, true],
    [3, 2, true],
    [10, 0, false],
  ])('checks %i staff against %i seats', async (staff_users, max_users, atLimit) => {
    vi.mocked(axios.get).mockResolvedValue({ data: { staff_users, max_users } });
    const quota = useStaffQuotaStore();
    expect(quota.canAddStaff).toBe(false);
    const refresh = quota.refresh();
    expect(quota.canAddStaff).toBe(false);
    await refresh;
    expect(quota.atLimit).toBe(atLimit);
    expect(quota.canAddStaff).toBe(!atLimit);
  });

  it('blocks staff when usage cannot be verified, and recovers on retry', async () => {
    const quota = useStaffQuotaStore();
    quota.usage = { staff_users: 1, max_users: 2 };
    vi.mocked(axios.get).mockRejectedValueOnce(new Error('offline'));
    await quota.refresh();
    expect(quota.canAddStaff).toBe(false);
    expect(quota.error).not.toBe('');
    vi.mocked(axios.get).mockResolvedValueOnce({ data: { staff_users: 1, max_users: 2 } });
    await quota.refresh();
    expect(quota.canAddStaff).toBe(true);
  });

  it.each([
    [true, 'https://accounts.example.com/account', true],
    [true, '', false],
    [false, 'https://accounts.example.com/account', false],
  ])('shows upgrades only for a hosted owner (%s, %s)', async (owner, url, visible) => {
    const auth = useAuthStore();
    auth.user = { is_initial_admin: owner, email: 'owner@example.com' } as User;
    const branding = useBrandingStore();
    await flushPromises();
    branding.branding.accountDashboardURL = url;
    useStaffQuotaStore().usage = { staff_users: 2, max_users: 2 };
    const wrapper = mount(StaffSeatUsage);
    expect(wrapper.text()).toContain('2 of 2');
    expect(wrapper.find('a').exists()).toBe(visible);
    expect(wrapper.findAll('a').at(0)?.attributes('href')).toBe(
      visible ? `${url}#email=owner%40example.com` : undefined,
    );
    expect(wrapper.findAll('a').at(0)?.attributes('target')).toBe(visible ? '_blank' : undefined);
    wrapper.unmount();
  });

  async function openModal(user?: User) {
    const wrapper = mount(UserModal, {
      props: { show: false, mode: user ? 'edit' : 'add', clients: [], user },
      global: {
        plugins: [OpenVue],
        stubs: {
          Dialog: { template: '<div><slot name="header" /><slot /></div>' },
          Button: { template: '<button><slot /></button>' },
          MultiSelect: true,
          Select: {
            props: ['modelValue', 'options', 'optionValue', 'inputId', 'disabled'],
            emits: ['update:modelValue'],
            template:
              '<select :id="inputId" :value="modelValue" :disabled="disabled" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="option in options" :key="option[optionValue]" :value="option[optionValue]">{{ option.label || option.name }}</option></select>',
          },
        },
      },
    });
    vi.mocked(axios.get).mockImplementation(async (url) => {
      if (url === '/api/access-roles')
        return {
          data: [
            {
              id: 'viewer',
              template_key: 'viewer',
              name: 'Viewer',
              audience: 'both',
              allow_workspace: true,
              allow_folder: true,
              allow_file: true,
              can_read: true,
            },
          ],
        };
      if (url === '/api/access-groups') return { data: [] };
      if (String(url).endsWith('/access-settings'))
        return {
          data: {
            profile: {
              user_type:
                user?.role === 'manager' || user?.role === 'admin' ? 'internal' : 'external',
              organization_role: user?.role === 'admin' ? 'administrator' : 'member',
              all_workspaces: user?.manager_full_access || false,
            },
            workspaces: (user?.client_ids || []).map((id) => ({
              client_id: id,
              role_id: 'viewer',
            })),
            group_ids: [],
          },
        };
      return { data: { staff_users: 2, max_users: 2 } };
    });
    await wrapper.setProps({ show: true });
    await flushPromises();
    return wrapper;
  }

  it('blocks staff submission but permits an external account without workspace access', async () => {
    const wrapper = await openModal();
    expect(wrapper.find('input[type="email"]').exists()).toBe(true);
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined();
    await wrapper.get('form').trigger('submit');
    expect(wrapper.emitted('submit')).toBeUndefined();
    await wrapper.get('select[id$="-type"]').setValue('external');
    await wrapper.get('input[type="text"]').setValue('Customer');
    await wrapper.get('input[type="email"]').setValue('customer@example.com');
    await wrapper.get('form').trigger('submit');
    expect(wrapper.emitted('submit')?.[0]).toEqual([
      {
        name: 'Customer',
        email: 'customer@example.com',
        user_type: 'external',
        organization_role: 'none',
        all_workspaces: false,
        workspace_access: [],
        group_ids: [],
      },
    ]);
    wrapper.unmount();
  });

  it('allows existing staff edits at the limit but blocks customer promotion', async () => {
    const staff = await openModal({
      role: 'manager',
      name: 'Staff',
      email: 'staff@example.com',
    } as User);
    expect(staff.get('button[type="submit"]').attributes('disabled')).toBeUndefined();
    await staff.get('form').trigger('submit');
    expect(staff.emitted('submit')).toHaveLength(1);
    expect(staff.emitted('submit')?.[0]).toEqual([
      expect.objectContaining({
        user_type: 'internal',
        organization_role: 'member',
        all_workspaces: false,
        workspace_access: [],
      }),
    ]);
    staff.unmount();
    const customer = await openModal({
      role: 'client',
      name: 'Customer',
      email: 'client@example.com',
    } as User);
    await customer.get('select[id$="-type"]').setValue('internal');
    await customer.get('select[id$="-organization"]').setValue('administrator');
    await customer.get('form').trigger('submit');
    expect(customer.emitted('submit')).toBeUndefined();
    customer.unmount();
  });

  it('preserves explicit manager full access and permits assigned client selection', async () => {
    const wrapper = await openModal({
      role: 'manager',
      name: 'Staff',
      email: 'staff@example.com',
      manager_full_access: true,
      client_ids: ['client-1'],
    } as User);
    expect(wrapper.findComponent(Checkbox).props('modelValue')).toBe(true);
    await wrapper.get('form').trigger('submit');
    expect(wrapper.emitted('submit')?.[0]).toEqual([
      expect.objectContaining({
        all_workspaces: true,
        workspace_access: [{ client_id: 'client-1', role_id: 'viewer' }],
      }),
    ]);
    wrapper.findComponent(Checkbox).vm.$emit('update:modelValue', false);
    await flushPromises();
    expect(wrapper.findComponent({ name: 'MultiSelect' }).exists()).toBe(true);
    wrapper.unmount();
  });

  it('offers managers only customer creation, never staff escalation', async () => {
    useAuthStore().user = { id: 'manager', role: 'manager' } as User;
    const wrapper = await openModal();
    expect(wrapper.find('option[value="internal"]').exists()).toBe(false);
    expect(wrapper.find('select[id$="-organization"]').exists()).toBe(false);
    expect(wrapper.get('select[id$="-type"]').element.value).toBe('external');
    expect(wrapper.find('input[type="email"]').exists()).toBe(true);
    wrapper.unmount();
  });

  it.each(['client', 'customer'])(
    'edits an item-only %s without assigning a client',
    async (role) => {
      const wrapper = await openModal({
        role,
        name: 'Recipient',
        email: 'recipient@example.com',
        client_ids: [],
      } as User);
      await wrapper.get('form').trigger('submit');
      expect(wrapper.emitted('submit')?.[0]).toEqual([
        expect.objectContaining({ user_type: 'external', workspace_access: [] }),
      ]);
      wrapper.unmount();
    },
  );

  it('rechecks seats after upgrading in another tab without losing the draft', async () => {
    const wrapper = await openModal();
    const quota = useStaffQuotaStore();
    quota.usage = { staff_users: 1, max_users: 2 };
    await flushPromises();
    await wrapper.get('input[type="text"]').setValue('New staff');
    await wrapper.get('input[type="email"]').setValue('new@example.com');
    quota.usage = { staff_users: 2, max_users: 2 };
    await flushPromises();
    expect(wrapper.find('input[type="email"]').exists()).toBe(true);
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined();
    vi.mocked(axios.get).mockResolvedValue({ data: { staff_users: 2, max_users: 5 } });
    window.dispatchEvent(new Event('focus'));
    await flushPromises();
    expect(wrapper.get('input[type="text"]').element.value).toBe('New staff');
    expect(wrapper.get('input[type="email"]').element.value).toBe('new@example.com');
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeUndefined();
    wrapper.unmount();
  });
});
