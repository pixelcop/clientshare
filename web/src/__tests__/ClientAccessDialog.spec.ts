import { flushPromises, mount } from '@vue/test-utils';
import OpenVue from 'openvue/config';
import { createPinia, setActivePinia } from 'pinia';
import { describe, expect, it } from 'vitest';

import ClientAccessDialog from '@/components/ClientAccessDialog.vue';
import { useAuthStore } from '@/stores/auth';
import type { User } from '@/types/models';

describe('client access entry', () => {
  it.each([
    ['admin', true, true],
    ['manager', true, false],
    ['client', false, false],
    ['link', false, false],
  ])('preserves existing management gates for %s', async (role, people, links) => {
    setActivePinia(createPinia());
    useAuthStore().user = { id: 'user', role } as User;
    const wrapper = mount(ClientAccessDialog, {
      props: { visible: true, clientId: 'client', clientName: 'Acme' },
      global: {
        plugins: [OpenVue],
        stubs: { teleport: true, RouterLink: { template: '<a><slot /></a>' } },
      },
    });
    await flushPromises();
    expect(wrapper.text()).toContain('Scope: entire client workspace');
    expect(wrapper.text()).toContain('not just the current folder or file');
    expect(wrapper.text().includes('Manage people')).toBe(people);
    expect(wrapper.text().includes('Manage links')).toBe(links);
    wrapper.unmount();
  });
});
