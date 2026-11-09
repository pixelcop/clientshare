import { flushPromises, mount } from '@vue/test-utils';
import axios from 'axios';
import OpenVue from 'openvue/config';
import Select from 'openvue/select';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import SharingPanel from '@/components/SharingPanel.vue';
import type { AccessInvitation, SecureLink } from '@/types/models';

const capabilities = { can_read: true, can_upload: false, can_delete: false, can_share: true };
const scope = { kind: 'item' as const, id: 'file', name: 'Selected document', itemType: 'file' };
const access = { capabilities, grants: [], invitations: [], links: [] };

describe('exact-scope sharing panel', () => {
  beforeEach(() => {
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
    vi.spyOn(axios, 'get').mockImplementation(async (url) => ({
      data: String(url).endsWith('/access') ? access : [],
    }));
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
    });
  });

  it('separates People and Links without losing a draft, and resets tabs for a new target', async () => {
    const wrapper = mount(SharingPanel, { props: { scope }, global: { plugins: [OpenVue] } });
    await flushPromises();
    const tabs = wrapper.findAll('[role="tab"]');
    expect(tabs.map((tab) => tab.text())).toEqual(['People', 'Links']);
    expect(tabs[0].attributes('aria-selected')).toBe('true');
    await wrapper.get('input#item-file-email').setValue('draft@example.test');
    await tabs[1].trigger('click');
    expect(tabs[1].attributes('aria-selected')).toBe('true');
    expect(wrapper.findAll('[role="tabpanel"]')[0].isVisible()).toBe(false);
    await tabs[0].trigger('click');
    expect((wrapper.get('input#item-file-email').element as HTMLInputElement).value).toBe(
      'draft@example.test',
    );
    await tabs[1].trigger('click');
    await wrapper.setProps({
      scope: { kind: 'item', id: 'folder', name: 'Reports', itemType: 'folder' },
    });
    await flushPromises();
    expect(wrapper.findAll('[role="tab"]')[0].attributes('aria-selected')).toBe('true');
    expect(wrapper.text()).toContain('Other folders in this client are not included.');
    expect((wrapper.get('input#item-folder-email').element as HTMLInputElement).value).toBe('');
    wrapper.unmount();
  });

  it('uses the server ceiling, keeps files read-only and reports disabled invitation delivery truthfully', async () => {
    const invitation = {
      id: 'invitation',
      email: 'recipient@example.test',
      expires_at: new Date(Date.now() + 86400000).toISOString(),
    } as AccessInvitation;
    const post = vi
      .spyOn(axios, 'post')
      .mockResolvedValueOnce({ data: invitation })
      .mockRejectedValueOnce({
        isAxiosError: true,
        response: { status: 503, data: { error: 'Email is unavailable' } },
      });
    const wrapper = mount(SharingPanel, { props: { scope }, global: { plugins: [OpenVue] } });
    await flushPromises();
    expect(wrapper.text()).toContain('Sharing: this file only');
    expect(wrapper.find('input[id$="-upload"]').exists()).toBe(false);
    expect(wrapper.find('input[id$="-delete"]').exists()).toBe(false);
    expect(wrapper.findComponent(Select).props('options')).toHaveLength(1);
    expect(wrapper.findComponent(Select).props('ariaLabelledby')).toBe('item-file-mode-label');
    expect(wrapper.get('#item-file-mode-label').text()).toBe('Anyone with this link can');
    await wrapper.get('input#item-file-email').setValue('recipient@example.test');
    await wrapper.findAll('form')[0].trigger('submit');
    await flushPromises();
    expect(post).toHaveBeenNthCalledWith(
      1,
      '/api/files/file/invitations',
      expect.objectContaining({
        email: invitation.email,
        capabilities: { ...capabilities, can_share: false },
        expires_at: null,
      }),
      expect.objectContaining({ headers: expect.any(Object) }),
    );
    expect(post.mock.calls[1][0]).toBe('/api/invitations/invitation/email');
    expect(wrapper.text()).toContain('Invitation saved. Email is unavailable');
    expect(wrapper.text()).not.toContain('Invitation email queued');
    wrapper.unmount();
  });

  it('replays a lost creation response and retries email with the same link and request key', async () => {
    const link = {
      id: 'link',
      file_id: 'file',
      access_type: 'read',
      link_url: 'https://portal.example.test/links/existing-token',
      expires_at: new Date(Date.now() + 86400000).toISOString(),
    } as SecureLink;
    const post = vi
      .spyOn(axios, 'post')
      .mockRejectedValueOnce(new Error('Lost creation response'))
      .mockResolvedValueOnce({ data: link })
      .mockRejectedValueOnce(new Error('Email unavailable'))
      .mockResolvedValueOnce({ data: { status: 'queued' } });
    const wrapper = mount(SharingPanel, {
      attachTo: document.body,
      props: { scope },
      global: { plugins: [OpenVue] },
    });
    await flushPromises();
    await wrapper.get('[role="tab"][aria-selected="false"]').trigger('click');
    const form = wrapper.findAll('form')[1];
    await form.trigger('submit');
    await flushPromises();
    await form.trigger('submit');
    await flushPromises();
    expect(post.mock.calls[0][0]).toBe('/api/files/file/links');
    expect(post.mock.calls[1]).toEqual(post.mock.calls[0]);
    expect(wrapper.text()).toContain('Link ready');
    expect(document.activeElement?.textContent).toContain('Link ready');
    const copy = wrapper.findAll('button').find((button) => button.text() === 'Copy link')!;
    await copy.trigger('click');
    await flushPromises();
    expect(navigator.clipboard.writeText).toHaveBeenCalledExactlyOnceWith(link.link_url);
    await wrapper.get('input#item-file-link-email').setValue('reader@example.test');
    const emailForm = wrapper.get('input#item-file-link-email').element.closest('form')!;
    emailForm.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    await flushPromises();
    emailForm.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    await flushPromises();
    expect(post.mock.calls[2][0]).toBe('/api/links/link/email');
    expect(post.mock.calls[3]).toEqual(post.mock.calls[2]);
    expect(wrapper.text()).toContain('Link email is waiting to send.');
    wrapper.unmount();
  });

  it('edits a direct grant with its concurrency version and explains remaining inheritance', async () => {
    const grant = {
      id: 'grant',
      user_id: 'recipient',
      version: 7,
      recipient_email: 'recipient@example.test',
      can_read: true,
      can_upload: false,
      can_share: false,
      can_delete: false,
    };
    vi.mocked(axios.get).mockImplementation(async (url) => ({
      data: String(url).endsWith('/access')
        ? { ...access, inherits_client_access: true, grants: [grant] }
        : [],
    }));
    const patch = vi.spyOn(axios, 'patch').mockResolvedValue({ data: grant });
    const wrapper = mount(SharingPanel, { props: { scope }, global: { plugins: [OpenVue] } });
    await flushPromises();
    expect(wrapper.text()).toContain('You have access through the client');
    expect(wrapper.text()).toContain('Added here');
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'Edit')!
      .trigger('click');
    await wrapper.findAll('form')[0].trigger('submit');
    await flushPromises();
    expect(patch).toHaveBeenCalledExactlyOnceWith('/api/files/file/grants/grant', {
      version: 7,
      user_id: 'recipient',
      capabilities: { can_read: true, can_upload: false, can_share: false, can_delete: false },
      expires_at: null,
    });
    expect(wrapper.text()).toContain('Access through the client or another folder is unchanged.');
    wrapper.unmount();
  });
});
