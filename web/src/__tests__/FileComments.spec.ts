import { flushPromises, mount, type VueWrapper } from '@vue/test-utils';
import axios from 'axios';
import { Select } from 'openvue';
import OpenVue from 'openvue/config';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import FileComments from '@/components/FileComments.vue';
import { useAuthStore } from '@/stores/auth';
import type { FileComment, User } from '@/types/models';

const comment: FileComment = {
  id: '01',
  file_id: 'file',
  author_id: 'author',
  author_name: 'Alex',
  body: 'Please check page two.',
  visibility: 'shared',
  version: 1,
  created_at: '2026-11-17T12:00:00Z',
  updated_at: '2026-11-17T12:00:00Z',
  can_edit: true,
  can_view_history: true,
};
const page = (items = [comment], team = true, cursor = '') => ({
  items,
  can_internal: team,
  next_cursor: cursor,
});
async function render() {
  const wrapper = mount(FileComments, {
    props: { fileId: 'file' },
    global: { plugins: [OpenVue] },
    attachTo: document.body,
  });
  await flushPromises();
  return wrapper;
}

async function commentAction(wrapper: VueWrapper, label: string, index = 0) {
  const trigger = wrapper.findAll('.comment')[index].get('[aria-label="Comment actions"]');
  await trigger.trigger('click');
  await flushPromises();
  const popup = document.getElementById(trigger.attributes('aria-controls')!);
  const item = popup?.querySelector<HTMLElement>(`[role="menuitem"][aria-label="${label}"] a`);
  expect(item, `Menu action "${label}" should exist`).toBeTruthy();
  item!.click();
  await flushPromises();
}

describe('file conversation', () => {
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
    useAuthStore().user = { id: 'author', role: 'manager', session_id: 'session' } as User;
    vi.spyOn(axios, 'get').mockResolvedValue({ data: page() });
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    document.body.replaceChildren();
  });

  it('labels audiences, defaults staff to team-only, and renders text rather than HTML', async () => {
    vi.mocked(axios.get).mockResolvedValue({
      data: page([
        comment,
        { ...comment, id: '02', body: '<script>alert(1)</script>', visibility: 'team' },
      ]),
    });
    const wrapper = await render();
    expect(wrapper.text()).toContain('Team only');
    expect(wrapper.text()).toContain('Shared');
    expect(wrapper.text()).toContain('<script>alert(1)</script>');
    expect(wrapper.find('script').exists()).toBe(false);
    expect(wrapper.findComponent(Select).props('modelValue')).toBe('team');
    expect(wrapper.text()).toContain('Only staff with access to this file');
    expect(wrapper.text()).toContain('No email notifications');
    wrapper.unmount();
  });

  it('uses compact card headers and targets the correct comment from its action menu', async () => {
    vi.mocked(axios.get).mockResolvedValueOnce({
      data: page([{ ...comment, id: '02', body: 'Second comment' }, comment]),
    });
    const remove = vi.spyOn(axios, 'delete').mockResolvedValue({
      data: {
        ...comment,
        id: '02',
        body: '',
        deleted_at: comment.created_at,
        version: 2,
        can_edit: false,
      },
    });
    const wrapper = await render();
    expect(wrapper.findAll('article.comment-card')).toHaveLength(2);
    expect(wrapper.findAll('.comment-card-header [aria-label="Comment actions"]')).toHaveLength(2);
    expect(wrapper.findAll('.comment-avatar')[0]?.text()).toBe('A');
    expect(wrapper.find('.comment-tools').exists()).toBe(false);
    expect(document.querySelector('[role="menuitem"]')).toBeNull();
    await commentAction(wrapper, 'Remove comment', 1);
    expect(wrapper.findAll('.comment')[0]?.find('.comment-confirm').exists()).toBe(false);
    await wrapper.findAll('.comment')[1].get('.comment-confirm button').trigger('click');
    await flushPromises();
    expect(remove).toHaveBeenCalledWith('/api/files/file/comments/02', { data: { version: 1 } });
    wrapper.unmount();
  });

  it('allows shared discussion for client readers without internal controls or other authors’ tools', async () => {
    useAuthStore().user = { id: 'client', role: 'client', session_id: 'client-session' } as User;
    vi.mocked(axios.get).mockResolvedValue({
      data: page([{ ...comment, can_edit: false, can_view_history: false }], false),
    });
    const wrapper = await render();
    expect(wrapper.findComponent(Select).exists()).toBe(false);
    expect(wrapper.find('[aria-label="Edit comment"]').exists()).toBe(false);
    expect(wrapper.find('[aria-label="Comment actions"]').exists()).toBe(false);
    expect(wrapper.text()).not.toContain('History');
    expect(wrapper.text()).toContain('Add a shared comment');
    expect(wrapper.text()).toContain('Visible to signed-in people with access to this file');
    wrapper.unmount();
  });

  it('keeps a failed draft and retries using the same idempotency key', async () => {
    const post = vi
      .spyOn(axios, 'post')
      .mockRejectedValueOnce(new Error('uncertain response'))
      .mockResolvedValueOnce({
        data: { ...comment, id: '02', body: 'A note', visibility: 'team' },
      });
    const wrapper = await render();
    await wrapper.get('.comment-composer textarea').setValue('A note');
    await wrapper.get('.comment-composer').trigger('submit');
    await flushPromises();
    expect((wrapper.get('.comment-composer textarea').element as HTMLTextAreaElement).value).toBe(
      'A note',
    );
    expect(wrapper.get('[role="alert"]').text()).toContain('draft has been kept');
    await wrapper.get('.comment-composer').trigger('submit');
    await flushPromises();
    expect(post).toHaveBeenCalledTimes(2);
    expect(post.mock.calls[0]?.[2]).toEqual(post.mock.calls[1]?.[2]);
    expect(post.mock.calls[1]?.[1]).toEqual({ body: 'A note', visibility: 'team' });
    expect((wrapper.get('.comment-composer textarea').element as HTMLTextAreaElement).value).toBe(
      '',
    );
    expect(wrapper.findAll('.comment')).toHaveLength(2);
    wrapper.unmount();
  });

  it('edits with the captured version, confirms removal, and shows private history', async () => {
    const put = vi
      .spyOn(axios, 'put')
      .mockResolvedValue({ data: { ...comment, body: 'Updated', version: 2 } });
    const remove = vi.spyOn(axios, 'delete').mockResolvedValue({
      data: {
        ...comment,
        body: '',
        version: 3,
        deleted_at: '2026-11-17T13:00:00Z',
        can_edit: false,
      },
    });
    const wrapper = await render();
    await commentAction(wrapper, 'Edit comment');
    await wrapper.get('.comment-edit textarea').setValue('Updated');
    await wrapper.get('.comment-edit').trigger('submit');
    await flushPromises();
    expect(put).toHaveBeenCalledWith('/api/files/file/comments/01', {
      body: 'Updated',
      version: 1,
    });
    expect(wrapper.text()).toContain('Edited');
    await commentAction(wrapper, 'Remove comment');
    expect(remove).not.toHaveBeenCalled();
    await wrapper.get('.comment-confirm button').trigger('click');
    await flushPromises();
    expect(remove).toHaveBeenCalledWith('/api/files/file/comments/01', { data: { version: 2 } });
    expect(wrapper.text()).toContain('Comment removed');
    expect(wrapper.find('[aria-label="Edit comment"]').exists()).toBe(false);
    vi.mocked(axios.get).mockResolvedValueOnce({
      data: [
        {
          id: 'rev',
          comment_id: '01',
          version: 1,
          body: 'Original text',
          action: 'created',
          created_at: comment.created_at,
        },
      ],
    });
    await commentAction(wrapper, 'View history');
    await flushPromises();
    expect(wrapper.get('[aria-label="Your comment history"]').text()).toContain('Original text');
    wrapper.unmount();
  });

  it('keeps completed rows visible while refreshing, and prepends older pages', async () => {
    vi.mocked(axios.get).mockResolvedValueOnce({ data: page([comment], true, '01') });
    const wrapper = await render();
    const existing = wrapper.get('.comment').element;
    let resolve!: (value: { data: ReturnType<typeof page> }) => void;
    vi.mocked(axios.get).mockImplementationOnce(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    await wrapper.get('[aria-label="Refresh comments"]').trigger('click');
    expect(wrapper.get('.comment').element).toBe(existing);
    resolve({ data: page([comment], true, '01') });
    await flushPromises();
    vi.mocked(axios.get).mockResolvedValueOnce({
      data: page([{ ...comment, id: '00', body: 'Earlier comment' }]),
    });
    await wrapper.get('.earlier-comments').trigger('click');
    await flushPromises();
    expect(wrapper.findAll('.comment')[0]?.text()).toContain('Earlier comment');
    expect(axios.get).toHaveBeenLastCalledWith(
      '/api/files/file/comments',
      expect.objectContaining({ params: { before: '01' } }),
    );
    wrapper.unmount();
  });

  it('clears conversations on revoked access and ignores responses from a previous file', async () => {
    const wrapper = await render();
    let resolve!: (value: { data: ReturnType<typeof page> }) => void;
    vi.mocked(axios.get).mockImplementationOnce(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    await wrapper.get('[aria-label="Refresh comments"]').trigger('click');
    vi.mocked(axios.get).mockRejectedValueOnce({ isAxiosError: true, response: { status: 404 } });
    await wrapper.setProps({ fileId: 'other-file' });
    await flushPromises();
    resolve({ data: page() });
    await flushPromises();
    expect(wrapper.find('.comment').exists()).toBe(false);
    expect(wrapper.find('.comment-composer').exists()).toBe(false);
    expect(wrapper.get('[role="alert"]').text()).toContain('access may have changed');
    wrapper.unmount();
  });

  it('blocks oversized comments and preserves an edit on version conflict', async () => {
    const put = vi
      .spyOn(axios, 'put')
      .mockRejectedValue({ isAxiosError: true, response: { status: 409 } });
    const wrapper = await render();
    await wrapper.get('.comment-composer textarea').setValue('界'.repeat(4001));
    expect(
      wrapper.get('.comment-composer button[type="submit"]').attributes('disabled'),
    ).toBeDefined();
    await commentAction(wrapper, 'Edit comment');
    await wrapper.get('.comment-edit textarea').setValue('My edit');
    await wrapper.get('.comment-edit').trigger('submit');
    await flushPromises();
    expect(put).toHaveBeenCalledTimes(1);
    expect(wrapper.get('[role="alert"]').text()).toContain('changed elsewhere');
    expect((wrapper.get('.comment-edit textarea').element as HTMLTextAreaElement).value).toBe(
      'My edit',
    );
    wrapper.unmount();
  });

  it('never converts a pending team draft into a shared comment after staff access changes', async () => {
    const post = vi.spyOn(axios, 'post');
    const wrapper = await render();
    await wrapper.get('.comment-composer textarea').setValue('Private unfinished note');
    vi.mocked(axios.get).mockResolvedValueOnce({ data: page([comment], false) });
    await wrapper.get('[aria-label="Refresh comments"]').trigger('click');
    await flushPromises();
    expect(wrapper.text()).toContain('Team-note access changed');
    expect((wrapper.get('.comment-composer textarea').element as HTMLTextAreaElement).value).toBe(
      'Private unfinished note',
    );
    await wrapper.get('.comment-composer').trigger('submit');
    expect(post).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it('does not let an older refresh overwrite a completed edit', async () => {
    const wrapper = await render();
    let resolve!: (value: { data: ReturnType<typeof page> }) => void;
    vi.mocked(axios.get).mockImplementationOnce(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    await wrapper.get('[aria-label="Refresh comments"]').trigger('click');
    vi.spyOn(axios, 'put').mockResolvedValue({
      data: { ...comment, body: 'Latest edit', version: 2 },
    });
    await commentAction(wrapper, 'Edit comment');
    await wrapper.get('.comment-edit textarea').setValue('Latest edit');
    await wrapper.get('.comment-edit').trigger('submit');
    await flushPromises();
    resolve({ data: page() });
    await flushPromises();
    expect(wrapper.get('.comment-body').text()).toBe('Latest edit');
    wrapper.unmount();
  });

  it('drops cached team notes when an older-page response reports lost staff access', async () => {
    vi.mocked(axios.get).mockResolvedValueOnce({
      data: page(
        [
          { ...comment, id: '02', visibility: 'team', body: 'Previously visible team note' },
          comment,
        ],
        true,
        '01',
      ),
    });
    const wrapper = await render();
    expect(wrapper.text()).toContain('Previously visible team note');
    vi.mocked(axios.get).mockResolvedValueOnce({ data: page([{ ...comment, id: '00' }], false) });
    await wrapper.get('.earlier-comments').trigger('click');
    await flushPromises();
    expect(wrapper.text()).not.toContain('Previously visible team note');
    expect(wrapper.findComponent(Select).exists()).toBe(false);
    wrapper.unmount();
  });
});
