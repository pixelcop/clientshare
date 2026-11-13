import { flushPromises, mount } from '@vue/test-utils';
import axios from 'axios';
import OpenVue from 'openvue/config';
import ToastService from 'openvue/toastservice';
import { createPinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createMemoryHistory, createRouter } from 'vue-router';

import SettingsView from '@/views/SettingsView.vue';

const tenant = { site_title: 'Test workspace', secure_link_default_expiry_days: 90 };
async function open(tab?: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/settings', name: 'Settings', component: { template: '<div />' } }],
  });
  await router.push({ name: 'Settings', query: tab ? { tab } : {} });
  const wrapper = mount(SettingsView, {
    global: { plugins: [createPinia(), router, OpenVue, ToastService] },
  });
  await flushPromises();
  return { wrapper, router };
}

describe('settings tabs', () => {
  beforeEach(() => {
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
      vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })),
    );
    vi.spyOn(axios, 'get').mockImplementation(async (url) => {
      if (url === '/api/settings/tenant' || url === '/api/settings/branding') {
        return { data: tenant };
      }
      if (url === '/api/settings/system') {
        return { data: { tenancy_mode: 'hosted' } };
      }
      if (url === '/api/settings/usage') {
        return {
          data: {
            staff_users: 2,
            max_users: 5,
            storage_bytes: 1e9,
            max_storage_bytes: 5e9,
            reserved_bytes: 0,
          },
        };
      }
      if (url === '/api/email/queue') {
        return { data: { pending: [] } };
      }
      return { data: [] };
    });
  });
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('updates the URL when switching tabs and shows server-provided usage', async () => {
    const { wrapper, router } = await open();
    await wrapper
      .findAll('[role="tab"]')
      .find((tab) => tab.text() === 'Usage')!
      .trigger('click');
    await flushPromises();
    expect(router.currentRoute.value.query.tab).toBe('usage');
    expect(wrapper.get('[aria-label="Workspace usage"]').text()).toContain('Staff: 2 of 5');
    wrapper.unmount();
  });

  it('deep-links to roles and preserves branding drafts across tab switches', async () => {
    const { wrapper, router } = await open('access');
    expect(wrapper.get('[role="tabpanel"][data-p-active="true"]').text()).toContain(
      'Document access roles',
    );
    await router.replace({ query: { tab: 'branding' } });
    await wrapper.get('input[placeholder="ClientShare Portal"]').setValue('Unsaved brand');
    await router.replace({ query: { tab: 'usage' } });
    await router.replace({ query: { tab: 'branding' } });
    await flushPromises();
    expect(
      (wrapper.get('input[placeholder="ClientShare Portal"]').element as HTMLInputElement).value,
    ).toBe('Unsaved brand');
    const put = vi.spyOn(axios, 'put').mockResolvedValue({ data: {} });
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'Save Changes')!
      .trigger('click');
    await flushPromises();
    const payload = put.mock.calls[0][1] as FormData;
    expect(put.mock.calls[0][0]).toBe('/api/settings/tenant');
    expect(payload.get('site_title')).toBe('Unsaved brand');
    wrapper.unmount();
  });

  it('falls back to Branding for an unknown tab', async () => {
    const { wrapper } = await open('unknown');
    expect(wrapper.get('[role="tab"][aria-selected="true"]').text()).toBe('Branding');
    wrapper.unmount();
  });
});
