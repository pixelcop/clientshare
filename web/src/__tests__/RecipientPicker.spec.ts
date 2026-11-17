import { flushPromises, mount } from '@vue/test-utils';
import axios from 'axios';
import { AutoComplete } from 'openvue';
import OpenVue from 'openvue/config';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { reactive } from 'vue';

import RecipientPicker from '@/components/RecipientPicker.vue';
import { recipientContext } from '@/types/recipients';

const { auth } = vi.hoisted(() => ({
  auth: { user: { id: 'staff', tenant_id: 'tenant', role: 'manager' } },
}));
vi.mock('@/stores/auth', () => ({ useAuthStore: () => reactive(auth) }));
const contact = {
  id: 'person',
  name: 'Alex Smith',
  email: 'alex@example.test',
  clients: [{ id: 'client', name: 'Acme' }],
  current_client: true,
  workspace_access: true,
};
function picker() {
  return mount(RecipientPicker, {
    props: { inputId: 'recipient', clientId: 'client', modelValue: '', required: true },
    global: { plugins: [OpenVue] },
    attachTo: document.body,
  });
}

describe('recipient picker', () => {
  beforeEach(() => {
    auth.user.role = 'manager';
    vi.spyOn(axios, 'get').mockResolvedValue({ data: [contact] });
  });
  afterEach(() => {
    vi.restoreAllMocks();
    document.body.innerHTML = '';
  });

  it('defaults to client contacts and selection only fills the email', async () => {
    const post = vi.spyOn(axios, 'post');
    const wrapper = picker();
    const input = wrapper.get('input');
    expect(input.attributes('type')).toBe('email');
    expect(input.attributes('required')).toBeDefined();
    expect(input.attributes('aria-label')).toBe('Recipient');
    await input.trigger('focus');
    await flushPromises();
    expect(axios.get).toHaveBeenCalledWith(
      '/api/recipients',
      expect.objectContaining({
        params: { client_id: 'client', scope: 'client', search: '' },
      }),
    );
    expect(document.body.textContent).toContain('Workspace access');
    expect(document.body.textContent).toContain('Acme');
    (document.querySelector('[role="option"]') as HTMLElement).click();
    await flushPromises();
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['alex@example.test']);
    expect(wrapper.text()).toContain('Selecting a contact does not change access');
    expect(post).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it('requires an explicit scope change and preserves new typed emails', async () => {
    const wrapper = picker();
    await wrapper.get('button').trigger('click');
    await flushPromises();
    expect(axios.get).toHaveBeenLastCalledWith(
      '/api/recipients',
      expect.objectContaining({ params: { client_id: 'client', scope: 'all', search: '' } }),
    );
    await wrapper.get('input').setValue('new@example.test');
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['new@example.test']);
    expect(wrapper.text()).toContain('Across authorized clients');
    wrapper.unmount();
  });

  it('discards an in-flight wider lookup when client context changes', async () => {
    let resolve!: (value: { data: (typeof contact)[] }) => void;
    vi.mocked(axios.get).mockImplementationOnce(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    const wrapper = picker();
    wrapper.findComponent(AutoComplete).vm.$emit('complete', { query: '' });
    await wrapper.setProps({ clientId: 'another-client' });
    resolve({ data: [contact] });
    await flushPromises();
    expect(wrapper.findComponent(AutoComplete).props('suggestions')).toEqual([]);
    expect(wrapper.text()).toContain('Current client contacts');
    wrapper.unmount();
  });

  it('keeps manual entry working when lookup fails and hides directory controls for clients', async () => {
    vi.mocked(axios.get).mockRejectedValueOnce(new Error('offline'));
    const wrapper = picker();
    wrapper.findComponent(AutoComplete).vm.$emit('complete', { query: '' });
    await flushPromises();
    expect(wrapper.text()).toContain('You can still enter an email address');
    wrapper.unmount();
    auth.user.role = 'client';
    vi.mocked(axios.get).mockClear();
    const client = picker();
    client.findComponent(AutoComplete).vm.$emit('complete', { query: 'alex' });
    await flushPromises();
    expect(axios.get).not.toHaveBeenCalled();
    expect(client.find('button').exists()).toBe(false);
    client.unmount();
  });

  it('labels limited associations separately from workspace access', () => {
    expect(recipientContext({ ...contact, workspace_access: false }, 'client')).toContain(
      'Associated',
    );
    expect(
      recipientContext({ ...contact, workspace_access: false, current_client: false }, 'client'),
    ).toBe('Not in this client');
  });
});
