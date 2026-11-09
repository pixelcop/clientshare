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
    ['admin', true, true],
    ['manager', true, true],
    ['client', false, false],
    ['link', false, false],
  ])(
    'keeps advanced staff routes separate from server-bounded sharing for %s',
    async (role, people, links) => {
      vi.restoreAllMocks();
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
      expect(wrapper.text()).toContain('Sharing: entire client workspace');
      expect(wrapper.text().includes('Manage people')).toBe(people);
      expect(wrapper.text().includes('Manage links')).toBe(links);
      expect(wrapper.find('input[type="email"]').exists()).toBe(role !== 'link');
      wrapper.unmount();
    },
  );
});
