import { flushPromises, mount } from '@vue/test-utils';
import axios from 'axios';
import OpenVue from 'openvue/config';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import SecureLinkModal from '@/components/SecureLinkModal.vue';
import SecureLinkResult from '@/components/SecureLinkResult.vue';
import { useSecureLinksStore } from '@/stores/secureLinks';
import { formatLinkExpiry, isUsableLink, linkAccessLabel } from '@/types/linkAccess';
import type { SecureLink } from '@/types/models';

const link = {
  id: 'link',
  client_id: 'client',
  access_type: 'read',
  expires_at: new Date(Date.now() + 86400000).toISOString(),
  link_url: 'https://portal.example.test/links/token',
} as SecureLink;

describe('link creation and delivery', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
    setActivePinia(createPinia());
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })),
    );
  });

  it('names legacy/current modes and rejects revoked/invalid/expired links', () => {
    expect(linkAccessLabel('view')).toBe('View & download');
    expect(linkAccessLabel('write')).toBe('Upload only');
    expect(linkAccessLabel('readwrite')).toBe('View, download & upload');
    expect(isUsableLink(link)).toBe(true);
    expect(isUsableLink({ ...link, revoked_at: new Date().toISOString() })).toBe(false);
    expect(isUsableLink({ ...link, expires_at: 'invalid' })).toBe(false);
    expect(isUsableLink({ ...link, expires_at: '2000-01-01' })).toBe(false);
    expect(formatLinkExpiry(link.expires_at)).toContain(
      Intl.DateTimeFormat().resolvedOptions().timeZone,
    );
  });

  it('retains creation keys after a lost response, without duplicating the list on retry', async () => {
    const store = useSecureLinksStore();
    vi.spyOn(axios, 'get').mockResolvedValue({ data: [] });
    await store.fetchLinks('client');
    const post = vi
      .spyOn(axios, 'post')
      .mockRejectedValueOnce(new Error('Response lost'))
      .mockResolvedValue({ data: link });
    await expect(store.createLink('client', 'read', 7)).rejects.toThrow();
    await store.createLink('client', 'read', 7);
    expect(post.mock.calls[0][2]?.headers?.['Idempotency-Key']).toBe(
      post.mock.calls[1][2]?.headers?.['Idempotency-Key'],
    );
    expect(store.links).toHaveLength(1);
  });

  it('does not restore broad saved permissions into Collect or another scope', async () => {
    localStorage.setItem(
      'clientshare:secure-link-settings',
      JSON.stringify({ accessType: 'readwrite', sendEmail: true }),
    );
    const wrapper = mount(SecureLinkModal, {
      props: { show: true, embedded: true, targetName: 'Acme' },
      global: { plugins: [OpenVue] },
    });
    const radios = wrapper.findAll('input[type="radio"]');
    await radios[1].setValue(true);
    expect(wrapper.vm.getSettings().accessType).toBe('write');
    expect(wrapper.vm.getSettings().sendEmail).toBe(false);
    expect(wrapper.text()).toContain('Sharing: entire client workspace');
    expect(wrapper.text()).toContain('Anyone with this link');
    expect(wrapper.text()).toContain('Acme');
    const result = wrapper.vm.getSettings();
    expect(Date.parse(result.expiresAt)).toBeGreaterThan(Date.now());
    await wrapper.setProps({ show: false });
    await wrapper.setProps({ show: true, targetName: 'Other client' });
    expect(wrapper.vm.getSettings().accessType).toBe('read');
    wrapper.unmount();
  });

  it('retries email using the same link and key and calls queued only queued', async () => {
    const store = useSecureLinksStore();
    const send = vi
      .spyOn(store, 'emailLink')
      .mockRejectedValueOnce(new Error('Lost response'))
      .mockResolvedValue({ status: 'queued' });
    let wrapper = mount(SecureLinkResult, {
      props: { link, targetName: 'Acme' },
      global: { plugins: [OpenVue] },
    });
    await wrapper.get('input[type="email"]').setValue('recipient@example.test');
    await wrapper.get('form').trigger('submit');
    await flushPromises();
    expect(wrapper.text()).toContain('Retry email');
    expect(wrapper.find('input[readonly]').element).toHaveProperty('value', link.link_url);
    wrapper.unmount();
    wrapper = mount(SecureLinkResult, {
      props: { link, targetName: 'Acme' },
      global: { plugins: [OpenVue] },
    });
    expect(wrapper.get('input[type="email"]').element).toHaveProperty(
      'value',
      'recipient@example.test',
    );
    expect(wrapper.text()).toContain('Retry email');
    await wrapper.get('form').trigger('submit');
    await flushPromises();
    expect(send.mock.calls[0]).toEqual(send.mock.calls[1]);
    expect(send.mock.calls[0][0]).toBe(link.id);
    expect(wrapper.text()).toContain('Email is waiting to send.');
    expect(wrapper.text()).not.toContain('Email sent.');
    wrapper.unmount();
  });
});
