import { flushPromises, mount } from '@vue/test-utils';
import axios from 'axios';
import OpenVue from 'openvue/config';
import Select from 'openvue/select';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import SharingExpiry from '@/components/SharingExpiry.vue';
import SharingLinks from '@/components/SharingLinks.vue';
import SharingPanel from '@/components/SharingPanel.vue';
import SharingPeople from '@/components/SharingPeople.vue';
import type { AccessInvitation, SecureLink } from '@/types/models';

vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: null }) }));

const capabilities = { can_read: true, can_upload: false, can_delete: false, can_share: true };
const scope = { kind: 'item' as const, id: 'file', name: 'Selected document', itemType: 'file' };
const access = { capabilities, grants: [], invitations: [], links: [] };
const role = (
  id: string,
  name: string,
  read = true,
  upload = false,
  del = false,
  share = false,
) => ({
  id,
  name,
  audience: 'both',
  allow_workspace: true,
  allow_folder: true,
  allow_file: !upload,
  can_read: read,
  can_upload: upload,
  can_delete: del,
  can_invite_external: share,
  can_create_links: share,
  can_manage_access: share,
  legacy_sharing: true,
});
const catalog = [
  role('viewer', 'Viewer'),
  role('uploader', 'Uploader', false, true),
  role('contributor', 'Contributor', true, true),
  role('file-manager', 'File manager', true, true, true),
  role('client-contact', 'Client contact', true, true, false, true),
];

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
      data: url === '/api/access-roles' ? catalog : String(url).endsWith('/access') ? access : [],
    }));
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
    });
  });

  it('shares by email without a recipient-type selector or an identity-change hint', async () => {
    const post = vi.spyOn(axios, 'post').mockResolvedValue({
      data: { id: 'invite', email: 'teammate@example.test', status: 'queued' },
    });
    const wrapper = mount(SharingPanel, { props: { scope }, global: { plugins: [OpenVue] } });
    await flushPromises();
    expect(wrapper.text()).not.toContain('Recipient type');
    expect(wrapper.findAllComponents(Select)).toHaveLength(0);
    await wrapper.get('#item-file-email').setValue('teammate@example.test');
    await wrapper.get('.add-person').trigger('submit');
    await wrapper.get('[aria-label="Send invitation"]').trigger('click');
    await flushPromises();
    expect(post.mock.calls[0][0]).toBe('/api/files/file/invitations');
    expect(post.mock.calls[0][1]).toMatchObject({
      email: 'teammate@example.test',
      role_id: 'viewer',
    });
    expect(post.mock.calls[0][1]).not.toHaveProperty('user_type');
    wrapper.unmount();
  });

  it('opens links separately without losing typed email, and resets for a new target', async () => {
    const wrapper = mount(SharingPanel, { props: { scope }, global: { plugins: [OpenVue] } });
    await flushPromises();
    expect(wrapper.findComponent(SharingPeople).exists()).toBe(true);
    expect(wrapper.findComponent(SharingLinks).exists()).toBe(false);
    const requests = vi.mocked(axios.get).mock.calls.length;
    expect(wrapper.find('[role="tab"]').exists()).toBe(false);
    await wrapper.get('input#item-file-email').setValue('draft@example.test');
    await wrapper.get('[aria-label="Sharing links"]').trigger('click');
    expect(wrapper.findComponent(SharingPeople).exists()).toBe(false);
    expect(wrapper.findComponent(SharingLinks).exists()).toBe(true);
    expect(wrapper.find('#item-file-email').exists()).toBe(false);
    await wrapper.get('[aria-label="Back to sharing"]').trigger('click');
    expect((wrapper.get('input#item-file-email').element as HTMLInputElement).value).toBe(
      'draft@example.test',
    );
    expect(axios.get).toHaveBeenCalledTimes(requests);
    await wrapper.get('[aria-label="Sharing links"]').trigger('click');
    await wrapper.setProps({
      scope: { kind: 'item', id: 'folder', name: 'Reports', itemType: 'folder' },
    });
    await flushPromises();
    expect(wrapper.find('[aria-label="Sharing links"]').exists()).toBe(true);
    expect((wrapper.get('input#item-folder-email').element as HTMLInputElement).value).toBe('');
    wrapper.unmount();
  });

  it('preserves link settings across child remounts without reloading access', async () => {
    const wrapper = mount(SharingPanel, { props: { scope }, global: { plugins: [OpenVue] } });
    await flushPromises();
    const requests = vi.mocked(axios.get).mock.calls.length;
    await wrapper.get('[aria-label="Sharing links"]').trigger('click');
    const expiry = new Date(Date.now() + 14 * 86400000);
    wrapper.findComponent(SharingExpiry).vm.$emit('update:modelValue', expiry);
    await flushPromises();
    await wrapper.get('[aria-label="Back to sharing"]').trigger('click');
    await wrapper.get('[aria-label="Sharing links"]').trigger('click');
    expect(wrapper.findComponent(SharingExpiry).props('modelValue')).toEqual(expiry);
    expect(axios.get).toHaveBeenCalledTimes(requests);
    wrapper.unmount();
  });

  it('orders active links newest first, followed by inactive links newest first without a divider', async () => {
    const links = [
      { id: 'active-old', created_at: '2026-01-01T00:00:00Z' },
      { id: 'revoked-new', created_at: '2026-06-01T00:00:00Z', revoked_at: '2026-07-01T00:00:00Z' },
      { id: 'active-new', created_at: '2026-04-01T00:00:00Z' },
      { id: 'expired-old', created_at: '2026-02-01T00:00:00Z', expires_at: '2000-01-01T00:00:00Z' },
      { id: 'active-middle', created_at: '2026-03-01T00:00:00Z' },
    ];
    vi.mocked(axios.get).mockImplementation(async (url) => {
      if (url === '/api/access-roles') return { data: catalog };
      return { data: String(url).endsWith('/access') ? access : links };
    });
    const wrapper = mount(SharingPanel, { props: { scope }, global: { plugins: [OpenVue] } });
    await flushPromises();
    await wrapper.get('[aria-label="Sharing links"]').trigger('click');
    const session = wrapper.findComponent(SharingLinks).props('session');
    expect(session.visibleLinks.value.map((link) => link.id)).toEqual([
      'active-new',
      'active-middle',
      'active-old',
    ]);
    await wrapper.findAll('[role="tab"]')[1]!.trigger('click');
    await wrapper.get('#item-file-history').setValue(true);
    expect(session.visibleLinks.value.map((link) => link.id)).toEqual([
      'active-new',
      'active-middle',
      'active-old',
      'revoked-new',
      'expired-old',
    ]);
    expect(wrapper.findAll('.existing-links > .access-row')).toHaveLength(5);
    expect(wrapper.text()).not.toContain('No recorded opens doesn’t guarantee');
    await wrapper.get('#item-file-history').setValue(false);
    expect(wrapper.findAll('.existing-links > .access-row')).toHaveLength(3);
    wrapper.unmount();
  });

  it('formats expiry distance in minutes, hours and days and updates while open', async () => {
    vi.useFakeTimers({ toFake: ['Date', 'setInterval', 'clearInterval'] });
    vi.setSystemTime(new Date('2026-01-01T12:00:00Z'));
    const soon = new Date(Date.now() + 120_000).toISOString();
    vi.mocked(axios.get).mockImplementation(async (url) => {
      if (url === '/api/access-roles') return { data: catalog };
      return {
        data: String(url).endsWith('/access') ? access : [{ id: 'soon', expires_at: soon }],
      };
    });
    const wrapper = mount(SharingPanel, { props: { scope }, global: { plugins: [OpenVue] } });
    try {
      await flushPromises();
      await wrapper.get('[aria-label="Sharing links"]').trigger('click');
      const { linkExpiryText } = wrapper.findComponent(SharingLinks).props('session');
      expect(linkExpiryText(new Date(Date.now() + 7 * 86400000).toISOString())).toBe(
        'Expires in 7 days',
      );
      expect(linkExpiryText(new Date(Date.now() + 2 * 3600000).toISOString())).toBe(
        'Expires in 2 hours',
      );
      expect(linkExpiryText(new Date(Date.now() + 5 * 60000).toISOString())).toBe(
        'Expires in 5 minutes',
      );
      expect(linkExpiryText(new Date(Date.now() + 30000).toISOString())).toBe(
        'Expires in less than a minute',
      );
      expect(linkExpiryText(new Date(Date.now() - 2 * 3600000).toISOString())).toBe(
        'Expired 2 hours ago',
      );
      await wrapper.findAll('[role="tab"]')[1]!.trigger('click');
      expect(wrapper.get('.existing-links .access-row').text()).toContain('Expires in 2 minutes');
      await vi.advanceTimersByTimeAsync(60_000);
      expect(wrapper.get('.existing-links .access-row').text()).toContain('Expires in 1 minute');
      await vi.advanceTimersByTimeAsync(60_000);
      expect(wrapper.findAll('.existing-links .access-row')).toHaveLength(0);
      await wrapper.get('#item-file-history').setValue(true);
      expect(wrapper.get('.existing-links .access-row').text()).toContain(
        'Expired less than a minute ago',
      );
    } finally {
      wrapper.unmount();
      vi.useRealTimers();
    }
  });

  it('keeps sharing sessions isolated between mounted panels', async () => {
    const first = mount(SharingPanel, { props: { scope }, global: { plugins: [OpenVue] } });
    const second = mount(SharingPanel, {
      props: { scope: { ...scope, id: 'another-file' } },
      global: { plugins: [OpenVue] },
    });
    await flushPromises();
    await first.get('#item-file-email').setValue('draft@example.test');
    await first.get('[aria-label="Sharing links"]').trigger('click');
    expect(second.findComponent(SharingPeople).exists()).toBe(true);
    expect((second.get('input[type="email"]').element as HTMLInputElement).value).toBe('');
    second.vm.requestClose();
    expect(second.emitted('close')).toHaveLength(1);
    expect(first.emitted('close')).toBeUndefined();
    first.unmount();
    second.unmount();
  });

  it('separates creation and management, preserves their state, and puts sharing navigation in the footer', async () => {
    vi.mocked(axios.get).mockImplementation(async (url) => {
      if (url === '/api/access-roles') return { data: catalog };
      return {
        data: String(url).endsWith('/access')
          ? access
          : [{ id: 'expired', expires_at: '2000-01-01T00:00:00Z' }],
      };
    });
    const wrapper = mount(SharingPanel, { props: { scope }, global: { plugins: [OpenVue] } });
    await flushPromises();
    const requests = vi.mocked(axios.get).mock.calls.length;
    await wrapper.get('.sharing-footer [aria-label="Sharing links"]').trigger('click');
    expect(wrapper.get('.sharing-footer [aria-label="Back to sharing"]').exists()).toBe(true);
    expect(
      wrapper.findComponent(SharingLinks).find('[aria-label="Back to sharing"]').exists(),
    ).toBe(false);
    expect(wrapper.find('.link-composer').exists()).toBe(true);
    expect(wrapper.find('.existing-links').exists()).toBe(false);
    const expiry = new Date(Date.now() + 14 * 86400000);
    wrapper.findComponent(SharingExpiry).vm.$emit('update:modelValue', expiry);
    await wrapper.findAll('[role="tab"]')[1]!.trigger('click');
    expect(wrapper.find('.link-composer').exists()).toBe(false);
    expect(wrapper.find('.existing-links').exists()).toBe(true);
    await wrapper.get('#item-file-history').setValue(true);
    await wrapper.findAll('[role="tab"]')[0]!.trigger('click');
    expect(wrapper.findComponent(SharingExpiry).props('modelValue')).toEqual(expiry);
    await wrapper.findAll('[role="tab"]')[1]!.trigger('click');
    expect(wrapper.findAll('.existing-links .access-row')).toHaveLength(1);
    await wrapper.get('.sharing-footer [aria-label="Back to sharing"]').trigger('click');
    expect(wrapper.findComponent(SharingPeople).exists()).toBe(true);
    await wrapper.get('.sharing-footer [aria-label="Sharing links"]').trigger('click');
    expect(wrapper.findAll('[role="tab"]')[1]!.attributes('aria-selected')).toBe('true');
    expect(wrapper.findAll('.existing-links .access-row')).toHaveLength(1);
    expect(axios.get).toHaveBeenCalledTimes(requests);
    wrapper.unmount();
  });

  it.each([
    { state: 'empty', links: [], hasHistory: false },
    { state: 'active only', links: [{ id: 'active' }], hasHistory: false },
    {
      state: 'expired and revoked',
      links: [
        { id: 'active' },
        { id: 'expired', expires_at: '2000-01-01T00:00:00Z' },
        { id: 'revoked', revoked_at: '2020-01-01T00:00:00Z' },
      ],
      hasHistory: true,
    },
    {
      state: 'history only',
      links: [{ id: 'expired', expires_at: '2000-01-01T00:00:00Z' }],
      hasHistory: true,
    },
  ])('only offers link history when it exists: $state', async ({ links, hasHistory }) => {
    vi.mocked(axios.get).mockImplementation(async (url) => ({
      data:
        url === '/api/access-roles' ? catalog : String(url).endsWith('/access') ? access : links,
    }));
    const wrapper = mount(SharingPanel, { props: { scope }, global: { plugins: [OpenVue] } });
    await flushPromises();
    await wrapper.get('[aria-label="Sharing links"]').trigger('click');
    expect(wrapper.findComponent(SharingExpiry).props('compact')).toBe(false);
    expect(wrapper.findComponent(SharingExpiry).props('label')).toBe('Link expiry');
    expect(wrapper.get('.link-settings').text()).toContain('Link expiry');
    expect(wrapper.find('[aria-label="Existing links"]').exists()).toBe(false);
    await wrapper.findAll('[role="tab"]')[1]!.trigger('click');
    expect(wrapper.find('[aria-label="Existing links"]').exists()).toBe(true);
    expect(wrapper.find('#item-file-history').exists()).toBe(hasHistory);
    expect(wrapper.findAll('.existing-links .access-row')).toHaveLength(
      links.filter((link) => link.id === 'active').length,
    );
    expect(wrapper.find('.link-settings').exists()).toBe(false);
    if (hasHistory) {
      await wrapper.get('#item-file-history').setValue(true);
      expect(wrapper.findAll('.existing-links .access-row')).toHaveLength(links.length);
      expect(wrapper.text()).toContain('These links no longer work.');
      const inactive = wrapper
        .findAll('.existing-links .access-row')
        .filter((row) => /Expired|Revoked/.test(row.text()));
      inactive.forEach((row) => expect(row.find('button').exists()).toBe(false));
      await wrapper.get('#item-file-history').setValue(false);
      expect(wrapper.text()).not.toContain('These links no longer work.');
    }
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
    expect(wrapper.find('input[id$="-upload"]').exists()).toBe(false);
    expect(wrapper.find('input[id$="-delete"]').exists()).toBe(false);
    await wrapper.get('input#item-file-email').setValue('recipient@example.test');
    await wrapper.get('.add-person').trigger('submit');
    expect(wrapper.findComponent(Select).props('options')).toHaveLength(1);
    expect(wrapper.text()).not.toContain('This file only');
    await wrapper.get('[aria-label="Send invitation"]').trigger('click');
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
    await wrapper.get('[aria-label="Sharing links"]').trigger('click');
    expect(wrapper.findComponent(Select).props('ariaLabelledby')).toBe('item-file-mode-label');
    expect(wrapper.get('#item-file-mode-label').text()).toBe('Anyone with this link can');
    const form = wrapper.findAll('form')[0];
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
    await wrapper.get('[aria-label="Back to sharing"]').trigger('click');
    expect(wrapper.text()).not.toContain('Link email is waiting to send.');
    wrapper.unmount();
  });

  it('creates a link in place without reloading or remounting the links panel', async () => {
    const link = {
      id: 'created-link',
      access_type: 'read',
      link_url: 'https://portal.example.test/links/new',
      expires_at: new Date(Date.now() + 86400000).toISOString(),
    } as SecureLink;
    const post = vi.spyOn(axios, 'post').mockResolvedValue({ data: link });
    const wrapper = mount(SharingPanel, {
      attachTo: document.body,
      props: { scope },
      global: { plugins: [OpenVue] },
    });
    await flushPromises();
    await wrapper.get('[aria-label="Sharing links"]').trigger('click');
    const backButton = wrapper.get('[aria-label="Back to sharing"]').element;
    const requests = vi.mocked(axios.get).mock.calls.length;
    await wrapper.get('.link-composer').trigger('submit');
    await flushPromises();
    expect(post).toHaveBeenCalledTimes(1);
    expect(axios.get).toHaveBeenCalledTimes(requests);
    expect(wrapper.get('[aria-label="Back to sharing"]').element).toBe(backButton);
    expect(wrapper.find('[role="status"]').text()).not.toContain('Loading');
    expect(wrapper.findAll('.existing-links .access-row')).toHaveLength(0);
    expect(document.activeElement?.textContent).toContain('Link ready');
    expect(wrapper.find('[aria-label="Open recipient view"]').exists()).toBe(false);
    await wrapper.get('.link-address button').trigger('click');
    await flushPromises();
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(link.link_url);
    expect(wrapper.get('.copy-status').text()).toBe('Link copied.');
    await wrapper.findAll('[role="tab"]')[1]!.trigger('click');
    expect(wrapper.findAll('.existing-links .access-row')).toHaveLength(1);
    expect(wrapper.find('.link-address').exists()).toBe(false);
    wrapper.unmount();
  });

  it('copies an existing link directly without leaving management, and shows recorded opens', async () => {
    const links = [
      {
        id: 'used',
        access_type: 'read',
        link_url: 'https://portal.example.test/links/used',
        last_accessed: '2026-01-05T12:00:00Z',
      },
      {
        id: 'no-activity',
        access_type: 'read',
        link_url: 'https://portal.example.test/links/other',
      },
    ];
    vi.mocked(axios.get).mockImplementation(async (url) => {
      if (url === '/api/access-roles') return { data: catalog };
      return { data: String(url).endsWith('/access') ? access : links };
    });
    const post = vi.spyOn(axios, 'post');
    const wrapper = mount(SharingPanel, {
      attachTo: document.body,
      props: { scope },
      global: { plugins: [OpenVue] },
    });
    await flushPromises();
    await wrapper.get('[aria-label="Sharing links"]').trigger('click');
    await wrapper.findAll('[role="tab"]')[1]!.trigger('click');
    expect(wrapper.findAll('.link-usage')[0]!.text()).toContain('Last opened');
    expect(wrapper.findAll('.link-usage')[1]!.text()).toBe('No recorded opens');
    await wrapper.findAll('[aria-label="Copy"]')[0]!.trigger('click');
    await flushPromises();
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(links[0]!.link_url);
    expect(wrapper.get('[aria-label="Copied"]').exists()).toBe(true);
    expect(wrapper.findAll('[role="tab"]')[1]!.attributes('aria-selected')).toBe('true');
    expect(wrapper.find('#item-file-url').exists()).toBe(false);
    expect(post).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it('edits a direct grant with its concurrency version and explains remaining inheritance', async () => {
    const grant = {
      id: 'grant',
      user_id: 'recipient',
      version: 7,
      role_id: 'viewer',
      recipient_email: 'recipient@example.test',
      can_read: true,
      can_upload: false,
      can_share: false,
      can_delete: false,
    };
    vi.mocked(axios.get).mockImplementation(async (url) => ({
      data:
        url === '/api/access-roles'
          ? catalog
          : String(url).endsWith('/access')
            ? { ...access, inherits_client_access: true, grants: [grant] }
            : [],
    }));
    const patch = vi.spyOn(axios, 'patch').mockResolvedValue({ data: grant });
    const wrapper = mount(SharingPanel, { props: { scope }, global: { plugins: [OpenVue] } });
    await flushPromises();
    expect(wrapper.find('.person-form').exists()).toBe(false);
    expect(wrapper.get('[aria-label="Done"]').exists()).toBe(true);
    const expiry = new Date('2027-01-01T00:00:00Z');
    wrapper.findComponent(SharingExpiry).vm.$emit('update:modelValue', expiry);
    await flushPromises();
    expect(patch).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain('Pending changes');
    wrapper.findComponent(SharingExpiry).vm.$emit('update:modelValue', null);
    await flushPromises();
    expect(wrapper.get('[aria-label="Done"]').exists()).toBe(true);
    wrapper.findComponent(SharingExpiry).vm.$emit('update:modelValue', expiry);
    await flushPromises();
    await wrapper.get('[aria-label="Save"]').trigger('click');
    await flushPromises();
    expect(patch).toHaveBeenCalledExactlyOnceWith('/api/files/file/grants/grant', {
      version: 7,
      user_id: 'recipient',
      role_id: 'viewer',
      capabilities: { can_read: true, can_upload: false, can_share: false, can_delete: false },
      expires_at: expiry.toISOString(),
    });
    expect(wrapper.text()).toContain('Access through the client or another folder is unchanged.');
    wrapper.unmount();
  });

  it('adds recipients inline, keeps focus in the input, and hides links while composing', async () => {
    const wrapper = mount(SharingPanel, {
      attachTo: document.body,
      props: { scope },
      global: { plugins: [OpenVue] },
    });
    await flushPromises();
    expect(wrapper.find('.person-form').exists()).toBe(false);
    expect(wrapper.find('fieldset').exists()).toBe(false);
    expect(wrapper.find('.past-access').exists()).toBe(false);
    expect(wrapper.text()).not.toContain('Show history');
    await wrapper.get('#item-file-email').setValue('person@example.test');
    await wrapper.get('.add-person').trigger('submit');
    expect(wrapper.find('[role="tablist"]').exists()).toBe(false);
    expect(wrapper.find('[aria-label="People with access"]').exists()).toBe(false);
    expect(wrapper.findComponent(SharingExpiry).props('modelValue')).toBeNull();
    expect(wrapper.find('[aria-label="Sharing links"]').exists()).toBe(false);
    expect(document.activeElement?.id).toBe('item-file-email');
    expect(
      wrapper
        .findAllComponents(Select)[0]
        .props('options')
        .map((role: { label: string }) => role.label),
    ).toEqual(['Viewer']);
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'Cancel')!
      .trigger('click');
    expect(wrapper.get('#item-file-email').element).toBe(document.activeElement);
    expect((wrapper.get('#item-file-email').element as HTMLInputElement).value).toBe('');
    wrapper.unmount();
  });

  it('blurs the email input without focusing the panel after adding a recipient on mobile', async () => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn((query: string) => ({
        matches: query === '(max-width: 639px)',
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    );
    const wrapper = mount(SharingPanel, {
      attachTo: document.body,
      props: { scope },
      global: { plugins: [OpenVue] },
    });
    await flushPromises();
    const input = wrapper.get<HTMLInputElement>('#item-file-email');
    expect(input.attributes('autofocus')).toBeUndefined();
    input.element.focus();
    await input.setValue('mobile@example.test');
    await wrapper.get('.add-person').trigger('submit');
    await flushPromises();
    expect(document.activeElement).toBe(document.body);
    expect(wrapper.get('.sharing-panel').attributes('tabindex')).toBeUndefined();
    expect(wrapper.get('.draft-people').text()).toContain('mobile@example.test');
    expect(input.element.value).toBe('');
    wrapper.unmount();
  });

  it('uses mobile manage access and a person sheet while keeping role edits staged', async () => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn((query: string) => ({
        matches: query === '(max-width: 639px)',
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    );
    const grant = {
      id: 'mobile-grant',
      version: 4,
      role_id: 'viewer',
      recipient_name: 'Amber',
      recipient_email: 'amber@example.test',
      recipient_user_type: 'external',
      can_manage: true,
      can_read: true,
      can_upload: false,
      can_delete: false,
      can_share: false,
    };
    const roles = [role('viewer', 'Viewer'), role('reader', 'Reader')];
    vi.mocked(axios.get).mockImplementation(async (url) => {
      if (url === '/api/access-roles') {
        return { data: roles };
      }
      return { data: String(url).endsWith('/access') ? { ...access, grants: [grant] } : [] };
    });
    const patch = vi.spyOn(axios, 'patch').mockResolvedValue({ data: {} });
    const wrapper = mount(SharingPanel, {
      attachTo: document.body,
      props: { scope },
      global: { plugins: [OpenVue] },
    });
    await flushPromises();
    expect(wrapper.get('.mobile-sharing-header h2').text()).toBe('Share');
    expect(wrapper.find('[aria-label="Sharing links"]').exists()).toBe(true);
    await wrapper.get('[aria-label="Sharing links"]').trigger('click');
    expect(wrapper.get('.mobile-sharing-header h2').text()).toBe('Sharing links');
    await wrapper.get('[aria-label="Back to share"]').trigger('click');
    expect(wrapper.get('.mobile-sharing-header h2').text()).toBe('Share');
    expect(wrapper.find('[aria-label="People with access"]').exists()).toBe(false);
    await wrapper.get('.mobile-manage-access').trigger('click');
    expect(wrapper.get('.mobile-sharing-header h2').text()).toBe('Manage access');
    expect(wrapper.text()).not.toContain('General access');
    expect(wrapper.find('[aria-label="Sharing links"]').exists()).toBe(false);
    expect(wrapper.find('.add-person').exists()).toBe(false);
    await wrapper.get('[aria-label="Change access for amber@example.test"]').trigger('click');
    await flushPromises();
    const sheet = document.querySelector('.sharing-person-sheet')!;
    expect(sheet.textContent).toContain('amber@example.test');
    const reader = [...sheet.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('Reader'),
    )!;
    reader.click();
    await flushPromises();
    expect(wrapper.get('.mobile-person-role').text()).toBe('Reader');
    expect(patch).not.toHaveBeenCalled();
    await wrapper.get('[aria-label="Back to share"]').trigger('click');
    expect(wrapper.get('.mobile-sharing-header h2').text()).toBe('Share');
    await wrapper.get('.mobile-manage-access').trigger('click');
    expect(wrapper.get('.mobile-person-role').text()).toBe('Reader');
    await wrapper.get('[aria-label="Save"]').trigger('click');
    await flushPromises();
    expect(patch).toHaveBeenCalledWith(
      '/api/files/file/grants/mobile-grant',
      expect.objectContaining({ role_id: 'reader', version: 4 }),
    );
    wrapper.unmount();
  });

  it('preserves unusual existing permissions and expiry until a role is explicitly chosen', async () => {
    const grant = {
      id: 'custom',
      user_id: 'person',
      version: 3,
      role_id: 'legacy-delete',
      recipient_email: 'custom@example.test',
      can_read: false,
      can_upload: false,
      can_delete: true,
      can_share: false,
      expires_at: new Date(Date.now() + 86400000).toISOString(),
    };
    vi.mocked(axios.get).mockImplementation(async (url) => ({
      data:
        url === '/api/access-roles'
          ? [...catalog, role('legacy-delete', 'Legacy deletion', false, false, true)]
          : String(url).endsWith('/access')
            ? {
                ...access,
                capabilities: {
                  can_read: true,
                  can_upload: true,
                  can_delete: true,
                  can_share: true,
                },
                grants: [grant],
              }
            : [],
    }));
    const patch = vi.spyOn(axios, 'patch').mockResolvedValue({ data: grant });
    const wrapper = mount(SharingPanel, { props: { scope }, global: { plugins: [OpenVue] } });
    await flushPromises();
    expect(wrapper.findComponent(SharingExpiry).props('modelValue')?.toISOString()).toBe(
      grant.expires_at,
    );
    expect(
      wrapper
        .findAllComponents(Select)
        .find((s) => s.props('ariaLabel') === 'Role for custom@example.test')
        ?.props('modelValue'),
    ).toBe('legacy-delete');
    const expiry = new Date('2027-02-01T00:00:00Z');
    wrapper.findComponent(SharingExpiry).vm.$emit('update:modelValue', expiry);
    await flushPromises();
    await wrapper.get('[aria-label="Save"]').trigger('click');
    await flushPromises();
    expect(patch.mock.calls[0][1]).toEqual(
      expect.objectContaining({
        capabilities: { can_read: false, can_upload: false, can_delete: true, can_share: false },
        expires_at: expiry.toISOString(),
        version: 3,
      }),
    );
    wrapper.unmount();
  });

  it('separates past invitations from current people and never offers standalone delete or share', async () => {
    vi.mocked(axios.get).mockImplementation(async (url) => ({
      data:
        url === '/api/access-roles'
          ? catalog
          : String(url).endsWith('/access')
            ? {
                ...access,
                capabilities: {
                  can_read: true,
                  can_upload: true,
                  can_delete: true,
                  can_share: true,
                },
                invitations: [
                  { id: 'past', email: 'past@example.test', revoked_at: new Date().toISOString() },
                ],
              }
            : [],
    }));
    const wrapper = mount(SharingPanel, {
      props: { scope: { ...scope, itemType: 'folder' } },
      global: { plugins: [OpenVue] },
    });
    await flushPromises();
    expect(wrapper.get('.past-access').attributes('open')).toBeUndefined();
    expect(wrapper.get('.past-access').text()).toContain('Invitation revoked');
    await wrapper.get('#item-file-email').setValue('person@example.test');
    await wrapper.get('.add-person').trigger('submit');
    const select = wrapper.findAllComponents(Select)[0];
    expect(select.props('options').map((role: { label: string }) => role.label)).toEqual([
      'Viewer',
      'Uploader',
      'Contributor',
      'File manager',
      'Client contact',
    ]);
    select.vm.$emit('update:modelValue', 'uploader');
    await flushPromises();
    expect(wrapper.get('.draft-people').text()).not.toContain('existing files stay private');
    wrapper.unmount();
  });
});
