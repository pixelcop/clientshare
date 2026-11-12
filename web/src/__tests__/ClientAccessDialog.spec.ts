import { flushPromises, mount } from '@vue/test-utils';
import axios from 'axios';
import OpenVue from 'openvue/config';
import { createPinia, setActivePinia } from 'pinia';
import { describe, expect, it, vi } from 'vitest';

import ClientAccessDialog from '@/components/ClientAccessDialog.vue';
import { useAuthStore } from '@/stores/auth';
import type { User } from '@/types/models';

describe('client access entry', () => {
  it.each([
    ['admin', true],
    ['manager', true],
    ['client', false],
    ['link', false],
  ])('keeps the first view focused and sharing server-bounded for %s', async (role, people) => {
    vi.restoreAllMocks();
    vi.stubGlobal(
      'ResizeObserver',
      class {
        observe = vi.fn();

        unobserve = vi.fn();

        disconnect = vi.fn();
      },
    );
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({
        matches: false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        addListener: vi.fn(),
        removeListener: vi.fn(),
      })),
    );
    vi.spyOn(axios, 'get').mockImplementation(async (url) => {
      if (role === 'link') {
        throw new Error('Bearer links cannot manage sharing');
      }
      return {
        data: String(url).endsWith('/access')
          ? {
              capabilities: {
                can_read: true,
                can_upload: true,
                can_share: true,
                can_delete: false,
              },
              invitations: [],
              people: [],
              can_manage: people,
            }
          : [],
      };
    });
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
    expect(wrapper.text()).toContain('Entire client workspace');
    expect(wrapper.text()).not.toContain('Manage people');
    expect(
      wrapper.findAll('a').some((link) => link.text() === 'Manage links' && link.isVisible()),
    ).toBe(false);
    if (people) {
      await wrapper.get('[aria-label="Sharing links"]').trigger('click');
      expect(wrapper.findAll('a').some((link) => link.text() === 'Manage links')).toBe(false);
      await wrapper.findAll('[role="tab"]')[1]!.trigger('click');
    }
    expect(
      wrapper.findAll('a').some((link) => link.text() === 'Manage links' && link.isVisible()),
    ).toBe(people);
    expect(wrapper.find('input[type="email"]').exists()).toBe(role !== 'link' && !people);
    wrapper.unmount();
  });
});
