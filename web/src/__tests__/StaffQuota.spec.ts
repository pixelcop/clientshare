import { flushPromises, mount } from '@vue/test-utils';
import axios from 'axios';
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
        stubs: {
          Dialog: { template: '<div><slot name="header" /><slot /></div>' },
          Button: { template: '<button><slot /></button>' },
          MultiSelect: true,
        },
      },
    });
    vi.mocked(axios.get).mockResolvedValue({ data: { staff_users: 2, max_users: 2 } });
    await wrapper.setProps({ show: true });
    await flushPromises();
    return wrapper;
  }

  it('blocks staff before form entry but still permits customer invitations', async () => {
    const wrapper = await openModal();
    expect(wrapper.find('input[type="email"]').exists()).toBe(false);
    expect(wrapper.get('option[value="admin"]').attributes('disabled')).toBeDefined();
    expect(wrapper.get('option[value="manager"]').attributes('disabled')).toBeDefined();
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined();
    await wrapper.get('form').trigger('submit');
    expect(wrapper.emitted('submit')).toBeUndefined();
    await wrapper.get('select').setValue('client');
    await wrapper.get('input[type="text"]').setValue('Customer');
    await wrapper.get('input[type="email"]').setValue('customer@example.com');
    // Client assignment is still required for customer invitations.
    await wrapper.setProps({ defaultClientIds: ['client-1'] });
    await wrapper.setProps({ show: false });
    await wrapper.setProps({ defaultRole: 'client', show: true });
    await flushPromises();
    await wrapper.get('input[type="text"]').setValue('Customer');
    await wrapper.get('input[type="email"]').setValue('customer@example.com');
    await wrapper.get('form').trigger('submit');
    expect(wrapper.emitted('submit')?.[0]).toEqual([
      {
        name: 'Customer',
        email: 'customer@example.com',
        role: 'client',
        client_ids: ['client-1'],
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
    staff.unmount();
    const customer = await openModal({
      role: 'client',
      name: 'Customer',
      email: 'client@example.com',
    } as User);
    await customer.get('select').setValue('admin');
    await customer.get('form').trigger('submit');
    expect(customer.emitted('submit')).toBeUndefined();
    customer.unmount();
  });

  it('rechecks seats after upgrading in another tab without losing the draft', async () => {
    const wrapper = await openModal();
    const quota = useStaffQuotaStore();
    quota.usage = { staff_users: 1, max_users: 2 };
    await flushPromises();
    await wrapper.get('input[type="text"]').setValue('New staff');
    await wrapper.get('input[type="email"]').setValue('new@example.com');
    quota.usage = { staff_users: 2, max_users: 2 };
    await flushPromises();
    expect(wrapper.find('input[type="email"]').exists()).toBe(false);
    vi.mocked(axios.get).mockResolvedValue({ data: { staff_users: 2, max_users: 5 } });
    window.dispatchEvent(new Event('focus'));
    await flushPromises();
    expect(wrapper.get('input[type="text"]').element.value).toBe('New staff');
    expect(wrapper.get('input[type="email"]').element.value).toBe('new@example.com');
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeUndefined();
    wrapper.unmount();
  });
});
