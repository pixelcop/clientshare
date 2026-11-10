import { mount } from '@vue/test-utils';
import OpenVue from 'openvue/config';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createMemoryHistory, createRouter } from 'vue-router';

import Breadcrumb from '@/components/Breadcrumb.vue';
import { useClientsStore } from '@/stores/clients';
import { useFileManagerStore } from '@/stores/fileManager';
import type { Client, FileItem } from '@/types/models';
import { breadcrumbHiddenCount } from '@/utils/breadcrumb';

describe('breadcrumb overflow', () => {
  it('uses all available width without a folder-depth limit', () => {
    expect(breadcrumbHiddenCount([60, 80, 70, 90, 50, 80], 430, 40)).toBe(0);
  });

  it('hides only the oldest folders needed to fit, including the ellipsis', () => {
    expect(breadcrumbHiddenCount([60, 80, 70, 90], 240, 40)).toBe(2);
    expect(breadcrumbHiddenCount([60, 80, 70, 90], 280, 40)).toBe(1);
  });

  it('always retains the current folder, even with a very long name', () => {
    expect(breadcrumbHiddenCount([60, 80, 900], 100, 40)).toBe(2);
    expect(breadcrumbHiddenCount([900], 100, 40)).toBe(0);
  });

  it('supports root folders and resizing back to the full hierarchy', () => {
    expect(breadcrumbHiddenCount([], 100, 40)).toBe(0);
    expect(breadcrumbHiddenCount([80, 80, 80], 140, 40)).toBe(2);
    expect(breadcrumbHiddenCount([80, 80, 80], 240, 40)).toBe(0);
  });
});

describe('breadcrumb client location', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    useFileManagerStore().breadcrumbs = [{ id: 'root', filename: 'Root' } as FileItem];
    const clients = useClientsStore();
    clients.clients = [{ id: 'client', name: 'Acme', root_folder_id: 'root' } as Client];
    clients.clientId = 'client';
  });

  async function render() {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/folder/:folderId', component: { template: '<div />' } }],
    });
    await router.push('/folder/child');
    const wrapper = mount(Breadcrumb, {
      props: { heading: true },
      global: { plugins: [router, OpenVue] },
    });
    return { wrapper, router };
  }

  it('shows a plain visible heading without a pill or Home control at the root', async () => {
    const { wrapper } = await render();
    expect(wrapper.get('h1').text()).toBe('Acme');
    expect(wrapper.get('h1').classes()).not.toContain('sr-only');
    expect(wrapper.find('.client-link').exists()).toBe(false);
    expect(wrapper.find('.oi-home').exists()).toBe(false);
    wrapper.unmount();
  });

  it('shows the client as a text link to the root without a Home icon in a subfolder', async () => {
    useFileManagerStore().breadcrumbs.push({ id: 'child', filename: 'Taxes' } as FileItem);
    const { wrapper, router } = await render();
    const clientLink = wrapper.get('a.client-link');
    expect(clientLink.text()).toBe('Acme');
    expect(wrapper.find('.oi-home').exists()).toBe(false);
    expect(clientLink.attributes('href')).toBe('/folder/root');
    expect(wrapper.findAll('a')).toHaveLength(1);
    expect(wrapper.get('h1').text()).toBe('Taxes');
    expect(wrapper.get('h1').classes()).toContain('sr-only');
    await clientLink.trigger('click');
    await vi.waitFor(() => expect(router.currentRoute.value.path).toBe('/folder/root'));
    wrapper.unmount();
  });
});
