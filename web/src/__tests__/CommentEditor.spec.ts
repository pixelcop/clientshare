import { flushPromises, mount } from '@vue/test-utils';
import axios from 'axios';
import OpenVue from 'openvue/config';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import CommentBody from '@/components/CommentBody.vue';
import CommentEditor from '@/components/CommentEditor.vue';

function render(body = '**Ready**') {
  const wrapper = mount(CommentEditor, {
    props: {
      id: 'editor',
      fileId: 'file',
      sessionKey: 'session',
      visibility: 'shared',
      modelValue: body,
      'onUpdate:modelValue': (value: string) => {
        void wrapper.setProps({ modelValue: value });
      },
    },
    global: { plugins: [OpenVue] },
    attachTo: document.body,
  });
  return wrapper;
}
async function search(wrapper: ReturnType<typeof render>, value: string) {
  const input = wrapper.get('textarea');
  await input.setValue(value);
  (input.element as HTMLTextAreaElement).setSelectionRange(value.length, value.length);
  await input.trigger('input');
  await vi.advanceTimersByTimeAsync(200);
  await flushPromises();
}
describe('Markdown comments editor', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
    document.body.replaceChildren();
  });

  it('previews backend HTML and returns to the original Markdown source', async () => {
    const post = vi
      .spyOn(axios, 'post')
      .mockResolvedValue({ data: { body_html: '<p><strong>Ready</strong></p>' } });
    const wrapper = render('    code\n\n**Ready**\n');
    await wrapper.get('button[aria-pressed="false"]').trigger('click');
    await flushPromises();
    expect(post).toHaveBeenCalledWith(
      '/api/files/file/comments/preview',
      {
        body: '    code\n\n**Ready**\n',
        visibility: 'shared',
      },
      expect.any(Object),
    );
    expect(wrapper.get('.markdown-preview strong').text()).toBe('Ready');
    expect(wrapper.find('textarea').exists()).toBe(false);
    await wrapper.get('button[aria-pressed="false"]').trigger('click');
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe(
      '    code\n\n**Ready**\n',
    );
    wrapper.unmount();
  });
  it('has empty, loading and failed preview states without losing the draft', async () => {
    const wrapper = render('');
    const post = vi.spyOn(axios, 'post').mockRejectedValue(new Error('offline'));
    await wrapper.findAll('button')[1].trigger('click');
    expect(wrapper.text()).toContain('Nothing to preview');
    expect(post).not.toHaveBeenCalled();
    await wrapper.findAll('button')[0].trigger('click');
    await wrapper.get('textarea').setValue('Keep this');
    await wrapper.findAll('button')[1].trigger('click');
    await flushPromises();
    expect(wrapper.get('[role="alert"]').text()).toContain('Preview unavailable');
    await wrapper.findAll('button')[0].trigger('click');
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('Keep this');
    wrapper.unmount();
  });
  it('ignores late previews after file or audience changes', async () => {
    let resolve!: (result: unknown) => void;
    vi.spyOn(axios, 'post').mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    const wrapper = render();
    await wrapper.findAll('button')[1].trigger('click');
    expect(wrapper.text()).toContain('Loading preview');
    await wrapper.setProps({ visibility: 'team' });
    resolve({ data: { body_html: '<p>Old shared content</p>' } });
    await flushPromises();
    expect(wrapper.text()).not.toContain('Old shared content');
    expect(wrapper.find('.markdown-preview').exists()).toBe(false);
    wrapper.unmount();
  });
  it('searches @names and inserts ID-backed Markdown with keyboard selection', async () => {
    const get = vi
      .spyOn(axios, 'get')
      .mockResolvedValue({ data: [{ id: 'user-id', name: 'Chetan' }] });
    const wrapper = render('');
    await search(wrapper, 'Hello @chet');
    expect(get).toHaveBeenCalledWith(
      '/api/files/file/comments/mentions',
      expect.objectContaining({
        params: { search: 'chet', visibility: 'shared' },
      }),
    );
    expect(wrapper.get('[role="option"]').text()).toBe('@Chetan');
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter' });
    expect(wrapper.props('modelValue')).toBe('Hello [@Chetan](mention:user-id) ');
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false);
    wrapper.unmount();
  });
  it('cancels suggestions on Escape and discards stale session results', async () => {
    let resolve!: (result: unknown) => void;
    vi.spyOn(axios, 'get').mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    const wrapper = render('');
    await search(wrapper, '@chet');
    await wrapper.setProps({ sessionKey: 'new-session' });
    resolve({ data: [{ id: 'old', name: 'Private person' }] });
    await flushPromises();
    expect(wrapper.text()).not.toContain('Private person');
    vi.mocked(axios.get).mockResolvedValue({ data: [{ id: 'user', name: 'Chetan' }] });
    await search(wrapper, '@chet');
    await wrapper.get('textarea').trigger('keydown', { key: 'Escape' });
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false);
    wrapper.unmount();
  });
  it('does not search email addresses and reports no matches', async () => {
    const get = vi.spyOn(axios, 'get').mockResolvedValue({ data: [] });
    const wrapper = render('');
    await search(wrapper, 'mail@example');
    expect(get).not.toHaveBeenCalled();
    await search(wrapper, '@nobody');
    expect(wrapper.text()).toContain('No people available');
    wrapper.unmount();
  });
  it('escapes raw text when rendered HTML is unavailable', () => {
    const wrapper = mount(CommentBody, { props: { body: '<script>unsafe</script>' } });
    expect(wrapper.find('script').exists()).toBe(false);
    expect(wrapper.text()).toBe('<script>unsafe</script>');
    wrapper.unmount();
  });
});
