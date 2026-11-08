import { flushPromises, mount } from '@vue/test-utils';
import axios from 'axios';
import OpenVue from 'openvue/config';
import Select from 'openvue/select';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createMemoryHistory, createRouter } from 'vue-router';

import ClientWorkspaceSwitcher from '@/components/ClientWorkspaceSwitcher.vue';
import { useAuthStore } from '@/stores/auth';
import type { Client, User } from '@/types/models';

const first = { id: 'first', name: 'First workspace', root_folder_id: 'first-root' } as Client;
const second = { id: 'second', name: 'Second workspace', root_folder_id: 'second-root' } as Client;

async function setup(clientId: string | null = first.id) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/folder/:folderId', name: 'FolderView', component: { template: '<div />' } }],
  });
  await router.push('/folder/first-root');
  const wrapper = mount(ClientWorkspaceSwitcher, {
    props: { clientId },
    global: { plugins: [OpenVue, router] },
  });
  await flushPromises();
  return { wrapper, router };
}

describe('authorized client workspace switching', () => {
  beforeEach(() => {
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
    setActivePinia(createPinia());
    useAuthStore().user = { id: 'recipient', role: 'client' } as User;
    vi.spyOn(axios, 'get').mockResolvedValue({ data: [first, second] });
  });

  it.each(['client', 'customer'])(
    'restores %s workspace switching using authorized roots',
    async (role) => {
      useAuthStore().user!.role = role;
      const { wrapper, router } = await setup();
      const select = wrapper.findComponent(Select);
      expect(axios.get).toHaveBeenCalledExactlyOnceWith('/api/clients');
      expect(select.props('options')).toEqual([first, second]);
      expect(select.props('modelValue')).toBe(first.id);
      expect(wrapper.get(`#${select.props('ariaLabelledby')}`).text()).toBe('Client workspace');
      select.vm.$emit('update:modelValue', second.id);
      await flushPromises();
      expect(router.currentRoute.value.params.folderId).toBe(second.root_folder_id);
      await wrapper.setProps({ clientId: second.id });
      expect(select.props('modelValue')).toBe(second.id);
      wrapper.unmount();
    },
  );

  it('does not invent a membership for an item-only folder, but retains real workspace choices', async () => {
    const { wrapper, router } = await setup('item-only-client');
    const select = wrapper.findComponent(Select);
    expect(select.props('modelValue')).toBeNull();
    expect(select.props('options')).toEqual([first, second]);
    select.vm.$emit('update:modelValue', 'item-only-client');
    await flushPromises();
    expect(router.currentRoute.value.params.folderId).toBe(first.root_folder_id);
    wrapper.unmount();
  });

  it('hides the switcher for a named item-only account without memberships', async () => {
    vi.mocked(axios.get).mockResolvedValue({ data: [] });
    const { wrapper } = await setup('item-only-client');
    expect(wrapper.findComponent(Select).exists()).toBe(false);
    wrapper.unmount();
  });

  it.each(['link', 'admin', 'manager'])(
    'does not request client options for %s sessions',
    async (role) => {
      useAuthStore().user!.role = role;
      const { wrapper } = await setup();
      expect(axios.get).not.toHaveBeenCalled();
      expect(wrapper.findComponent(Select).exists()).toBe(false);
      wrapper.unmount();
    },
  );

  it('clears options on an identity change and ignores the previous request', async () => {
    let finish!: (value: { data: Client[] }) => void;
    vi.mocked(axios.get).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const { wrapper } = await setup();
    vi.mocked(axios.get).mockResolvedValue({ data: [] });
    useAuthStore().user = { id: 'other-recipient', role: 'client' } as User;
    await flushPromises();
    finish({ data: [first, second] });
    await flushPromises();
    expect(wrapper.findComponent(Select).exists()).toBe(false);
    wrapper.unmount();
  });

  it('offers retry after failure without stale workspace choices', async () => {
    vi.mocked(axios.get).mockRejectedValueOnce(new Error('Unavailable'));
    const { wrapper } = await setup();
    expect(wrapper.get('[role="alert"]').text()).toContain(
      'Client workspaces could not be loaded.',
    );
    expect(wrapper.findComponent(Select).props('options')).toEqual([]);
    await wrapper.get('button').trigger('click');
    await flushPromises();
    expect(wrapper.findComponent(Select).props('options')).toEqual([first, second]);
    wrapper.unmount();
  });
});
