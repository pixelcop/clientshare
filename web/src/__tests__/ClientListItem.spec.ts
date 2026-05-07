import { mount, RouterLinkStub } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';
import { defineComponent } from 'vue';
import { createMemoryHistory, createRouter } from 'vue-router';

import ClientListItem from '@/components/ClientListItem.vue';
import type { Client } from '@/types/models';

Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation((query) => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  })),
});

const splitButtonStub = defineComponent({
  name: 'SplitButton',
  props: {
    model: {
      type: Array,
      default: () => [],
    },
  },
  template: '<div class="split-button-stub" />',
});

const client: Client = {
  id: 'client-123',
  tenant_id: 'tenant-123',
  name: 'Acme',
  folder_path: '/acme',
  root_folder_id: 'folder-123',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

async function mountComponent() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'Home', component: { template: '<div />' } },
      { path: '/users', name: 'Users', component: { template: '<div />' } },
    ],
  });

  await router.push('/');
  await router.isReady();

  const wrapper = mount(ClientListItem, {
    props: {
      client,
      showLinks: false,
      showUsers: true,
    },
    global: {
      plugins: [router],
      stubs: {
        RouterLink: RouterLinkStub,
        SplitButton: splitButtonStub,
      },
    },
  });

  return { wrapper, router };
}

describe('ClientListItem', () => {
  it('includes an Add User action in the client menu', async () => {
    const { wrapper } = await mountComponent();

    const actions = wrapper.getComponent(splitButtonStub).props('model') as {
      label?: string;
      visible?: boolean;
      command?: () => void;
    }[];

    const addUserAction = actions.find((action) => action.label === 'Add User');

    expect(addUserAction).toBeDefined();
    expect(addUserAction?.visible).toBe(true);
  });

  it('emits add-user from the Add User action', async () => {
    const { wrapper, router } = await mountComponent();

    const actions = wrapper.getComponent(splitButtonStub).props('model') as {
      label?: string;
      visible?: boolean;
      command?: () => void;
    }[];
    const addUserAction = actions.find((action) => action.label === 'Add User');

    addUserAction?.command?.();

    expect(wrapper.emitted('add-user')).toEqual([[client]]);
    expect(router.currentRoute.value.name).toBe('Home');
  });
});
