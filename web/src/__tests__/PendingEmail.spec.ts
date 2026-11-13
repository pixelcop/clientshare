import { flushPromises, mount } from '@vue/test-utils';
import axios from 'axios';
import { Dialog, ToggleSwitch } from 'openvue';
import OpenVue from 'openvue/config';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import PendingEmail from '@/components/PendingEmail.vue';
import { useAuthStore } from '@/stores/auth';
import type { QueuedEmail, User } from '@/types/models';

describe('file email options', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setActivePinia(createPinia());
    useAuthStore().user = { id: 'admin', role: 'admin' } as User;
    vi.spyOn(axios, 'get').mockResolvedValue({ data: { pending: [] } });
  });

  function setup(props = {}) {
    return mount(PendingEmail, {
      props: { clientId: 'client', canManageQueue: true, canNotify: true, ...props },
      global: { plugins: [OpenVue], stubs: { teleport: true } },
    });
  }

  it('binds the notification preference without sending mail and locks it during uploads', async () => {
    const post = vi.spyOn(axios, 'post');
    const wrapper = setup();
    await wrapper.get('button').trigger('click');
    await flushPromises();
    const toggle = wrapper.getComponent(ToggleSwitch);
    expect(toggle.props('modelValue')).toBe(true);
    toggle.vm.$emit('update:modelValue', false);
    await flushPromises();
    expect(wrapper.emitted('update:notificationsEnabled')).toEqual([[false]]);
    expect(post).not.toHaveBeenCalled();
    await wrapper.setProps({ notificationsDisabled: true });
    expect(toggle.props('disabled')).toBe(true);
    wrapper.unmount();
  });

  it('shows the pending count and refreshes it when opening the queue', async () => {
    vi.mocked(axios.get).mockResolvedValue({ data: { pending: [{ id: 'one' }, { id: 'two' }] } });
    const wrapper = setup();
    await flushPromises();
    expect(wrapper.get('button').attributes('aria-label')).toBe('Email options, 2 pending');
    vi.mocked(axios.get).mockResolvedValue({ data: { pending: [] } });
    await wrapper.get('button').trigger('click');
    await flushPromises();
    await wrapper.get('.pending-email-action').trigger('click');
    await flushPromises();
    expect(wrapper.get('[aria-label="Pending email for this client"]').text()).toContain(
      'No pending emails for this client.',
    );
    expect(axios.get).toHaveBeenCalledTimes(2);
    expect(axios.get).toHaveBeenLastCalledWith('/api/email/queue', {
      params: { client_id: 'client' },
    });
    expect(wrapper.get('button').attributes('aria-label')).toBe('Email options, 0 pending');
    wrapper.unmount();
  });

  it('restores focus to the original options trigger after the pending dialog closes', async () => {
    const wrapper = setup();
    await flushPromises();
    const trigger = wrapper.get('button[aria-label="Email options, 0 pending"]');
    const focus = vi.spyOn(trigger.element as HTMLElement, 'focus');
    await trigger.trigger('click');
    await flushPromises();
    await wrapper.get('.pending-email-action').trigger('click');
    await flushPromises();
    focus.mockClear();
    wrapper.getComponent(Dialog).vm.$emit('after-hide');
    await flushPromises();
    expect(focus).toHaveBeenCalledOnce();
    wrapper.unmount();
  });

  it('does not inspect the queue without workspace permission, even for an admin', async () => {
    const wrapper = setup({ canManageQueue: false });
    await wrapper.get('button').trigger('click');
    await flushPromises();
    expect(axios.get).not.toHaveBeenCalled();
    expect(wrapper.find('.pending-email-action').exists()).toBe(false);
    expect(wrapper.findComponent(ToggleSwitch).exists()).toBe(true);
    wrapper.unmount();
  });

  it('keeps queue-only access when uploading is unavailable', async () => {
    const wrapper = setup({ canNotify: false });
    await wrapper.get('button').trigger('click');
    await flushPromises();
    expect(wrapper.find('.pending-email-action').exists()).toBe(true);
    expect(wrapper.findComponent(ToggleSwitch).exists()).toBe(false);
    wrapper.unmount();
  });

  it('hides email controls for customers and disables the trigger while loading', async () => {
    const wrapper = setup({ disabled: true });
    expect(wrapper.get('button').attributes('disabled')).toBeDefined();
    wrapper.unmount();

    vi.mocked(axios.get).mockClear();
    useAuthStore().user = { id: 'customer', role: 'customer' } as User;
    const customer = setup({ canNotify: false });
    await flushPromises();
    expect(customer.find('button').exists()).toBe(false);
    expect(axios.get).not.toHaveBeenCalled();
    customer.unmount();
  });

  it('discards an old client response when the destination changes', async () => {
    let finish!: (response: { data: { pending: QueuedEmail[] } }) => void;
    vi.mocked(axios.get).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const wrapper = setup();
    await wrapper.setProps({ clientId: 'other-client' });
    await flushPromises();
    finish({ data: { pending: [{ id: 'old' } as QueuedEmail] } });
    await flushPromises();
    expect(wrapper.get('button').attributes('aria-label')).toBe('Email options, 0 pending');
    expect(axios.get).toHaveBeenLastCalledWith('/api/email/queue', {
      params: { client_id: 'other-client' },
    });
    wrapper.unmount();
  });
});
