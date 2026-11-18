import type { Editor } from '@tiptap/core';
import { EditorContent } from '@tiptap/vue-3';
import { flushPromises, mount } from '@vue/test-utils';
import axios from 'axios';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import CommentEditor from '@/components/CommentEditor.vue';

import { editorGeometry } from './editorGeometry';

async function render(source = '') {
  const wrapper = mount(CommentEditor, {
    props: {
      id: 'editor',
      fileId: 'file',
      sessionKey: 'session',
      visibility: 'shared',
      modelValue: source,
      'onUpdate:modelValue': (value: string) => {
        void wrapper.setProps({ modelValue: value });
      },
    },
    attachTo: document.body,
  });
  await flushPromises();
  return wrapper;
}
function instance(wrapper: Awaited<ReturnType<typeof render>>): Editor {
  return wrapper.findComponent(EditorContent).props('editor')!;
}
async function mention(wrapper: Awaited<ReturnType<typeof render>>, text = '@') {
  const editor = instance(wrapper);
  editor.view.focus();
  editor.commands.setContent('', { contentType: 'markdown' });
  editor.commands.insertContent(text);
  await vi.advanceTimersByTimeAsync(250);
  await flushPromises();
}
describe('rich Markdown comment editor', () => {
  let restoreGeometry: () => void;
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.useFakeTimers();
    restoreGeometry = editorGeometry();
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    restoreGeometry();
    document.body.replaceChildren();
  });

  it('loads existing Markdown as rich text without rewriting the original source', async () => {
    const source =
      '## Heading\n\n**Ready** and *reviewed*\n\n- One\n- Two\n\n`code`\n\n[@Chetan](mention:user)\n';
    const wrapper = await render(source);
    expect(wrapper.get('[role="textbox"] strong').text()).toBe('Ready');
    expect(wrapper.get('[role="textbox"] em').text()).toBe('reviewed');
    expect(wrapper.get('[role="textbox"] .comment-mention').text()).toBe('@Chetan');
    expect(wrapper.props('modelValue')).toBe(source);
    expect(wrapper.emitted('update:modelValue')).toBeUndefined();
    wrapper.unmount();
  });

  it('serializes edits and mentions as Markdown, not HTML', async () => {
    const wrapper = await render('Original');
    instance(wrapper).commands.setContent('**Edited** [@Chetan](mention:user)', {
      contentType: 'markdown',
    });
    await flushPromises();
    expect(wrapper.props('modelValue')).toBe('**Edited** [@Chetan](mention:user)');
    expect(
      (wrapper.get('[role="textbox"] .comment-mention').element as HTMLElement).contentEditable,
    ).toBe('false');
    expect(wrapper.props('modelValue')).not.toContain('<');
    wrapper.unmount();
  });

  it('opens on bare @ and inserts an inline rich mention with Enter', async () => {
    const get = vi.spyOn(axios, 'get').mockResolvedValue({
      data: [
        { id: 'alex', name: 'Alex', email: 'alex@example.test' },
        { id: 'chetan', name: 'Chetan', email: 'chetan@example.test' },
      ],
    });
    const wrapper = await render();
    await mention(wrapper);
    expect(get).toHaveBeenLastCalledWith(
      '/api/files/file/comments/mentions',
      expect.objectContaining({ params: { search: '', visibility: 'shared' } }),
    );
    expect(wrapper.findAll('[role="option"]')).toHaveLength(2);
    expect(wrapper.get('[role="textbox"]').attributes('aria-activedescendant')).toBe(
      'editor-mention-0',
    );
    await wrapper.get('[role="textbox"]').trigger('keydown', { key: 'ArrowDown' });
    expect(wrapper.get('[aria-selected="true"]').text()).toContain('@Chetan');
    expect(wrapper.get('[role="textbox"]').attributes('aria-activedescendant')).toBe(
      'editor-mention-1',
    );
    await wrapper.get('[role="textbox"]').trigger('keydown', { key: 'Enter' });
    await flushPromises();
    expect(wrapper.get('[role="textbox"] .comment-mention').text()).toBe('@Chetan');
    expect(wrapper.props('modelValue')).toContain('[@Chetan](mention:chetan)');
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it('supports pointer selection and keeps the identity through an edit round trip', async () => {
    vi.spyOn(axios, 'get').mockResolvedValue({ data: [{ id: 'user', name: 'Alex [Team]' }] });
    const wrapper = await render();
    await mention(wrapper, '@alex');
    await wrapper.get('[role="option"] button').trigger('click');
    await flushPromises();
    const source = wrapper.props('modelValue');
    expect(source).toContain('mention:user');
    expect(wrapper.get('.comment-mention').text()).toBe('@Alex [Team]');
    wrapper.unmount();
    const edited = await render(source);
    expect(edited.get('.comment-mention').text()).toBe('@Alex [Team]');
    edited.unmount();
  });

  it('dismisses on Escape without reopening in the same trigger context', async () => {
    vi.spyOn(axios, 'get').mockResolvedValue({ data: [{ id: 'user', name: 'Chetan' }] });
    const wrapper = await render();
    await mention(wrapper);
    await wrapper.get('[role="textbox"]').trigger('keydown', { key: 'Escape' });
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false);
    instance(wrapper).commands.insertContent('c');
    await vi.advanceTimersByTimeAsync(250);
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it('does not expose stale suggestions after an audience or session change', async () => {
    let resolve!: (value: unknown) => void;
    vi.spyOn(axios, 'get').mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    const wrapper = await render();
    await mention(wrapper);
    await wrapper.setProps({ visibility: 'team', sessionKey: 'new-session' });
    resolve({ data: [{ id: 'private', name: 'Private person' }] });
    await flushPromises();
    expect(wrapper.text()).not.toContain('Private person');
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it('handles empty and failed mention searches without blocking the draft', async () => {
    const get = vi.spyOn(axios, 'get').mockResolvedValue({ data: [] });
    const wrapper = await render();
    await mention(wrapper);
    expect(wrapper.text()).toContain('No people available');
    get.mockRejectedValueOnce(new Error('offline'));
    await mention(wrapper, '@chet');
    expect(wrapper.text()).toContain('Mention search unavailable');
    expect(wrapper.props('modelValue')).toBe('@chet');
    wrapper.unmount();
  });

  it('does not search email addresses or code, and respects read-only mode', async () => {
    const get = vi.spyOn(axios, 'get').mockResolvedValue({ data: [] });
    const wrapper = await render();
    await mention(wrapper, 'mail@example');
    expect(get).not.toHaveBeenCalled();
    instance(wrapper).commands.setContent('`@chet`', { contentType: 'markdown' });
    instance(wrapper).commands.setTextSelection(3);
    await vi.advanceTimersByTimeAsync(250);
    expect(get).not.toHaveBeenCalled();
    await wrapper.setProps({ readonly: true });
    expect(wrapper.get('[role="textbox"]').attributes('contenteditable')).toBe('false');
    wrapper.unmount();
  });

  it('explains the character limit when exceeded and clears the error after correction', async () => {
    const wrapper = await render();
    expect(wrapper.find('[role="alert"]').exists()).toBe(false);
    instance(wrapper).commands.setContent('界'.repeat(4001), { contentType: 'markdown' });
    await flushPromises();
    expect(wrapper.get('[role="alert"]').text()).toContain('4,000 characters, including Markdown');
    expect(wrapper.get('[role="textbox"]').attributes('aria-describedby')).toBe('editor-limit');
    expect(wrapper.get('[role="textbox"]').attributes('aria-invalid')).toBe('true');
    instance(wrapper).commands.setContent('Short enough', { contentType: 'markdown' });
    await flushPromises();
    expect(wrapper.find('[role="alert"]').exists()).toBe(false);
    expect(wrapper.get('[role="textbox"]').attributes('aria-invalid')).toBe('false');
    wrapper.unmount();
  });

  it('does not render raw HTML, scripts, tracking images or unsafe links from Markdown', async () => {
    const wrapper = await render(
      '<script>alert(1)</script>\n\n![tracking](https://evil.test/pixel)\n\n![](https://evil.test/empty)\n\n[link](javascript:alert%281%29)',
    );
    expect(wrapper.find('script').exists()).toBe(false);
    expect(wrapper.find('img').exists()).toBe(false);
    expect(wrapper.find('a[href^="javascript:"]').exists()).toBe(false);
    expect(wrapper.get('[role="textbox"]').text()).toContain('tracking');
    wrapper.unmount();
  });

  it('supports formatting undo/redo and copies Markdown with mention IDs', async () => {
    const wrapper = await render('Ready [@Chetan](mention:user)');
    const current = instance(wrapper);
    current.commands.setTextSelection({ from: 1, to: 6 });
    current.commands.toggleBold();
    await flushPromises();
    expect(wrapper.props('modelValue')).toContain('**Ready**');
    current.commands.undo();
    await flushPromises();
    expect(wrapper.props('modelValue')).toBe('Ready [@Chetan](mention:user)');
    current.commands.redo();
    await flushPromises();
    expect(wrapper.props('modelValue')).toContain('**Ready**');
    const slice = current.state.doc.slice(0, current.state.doc.content.size);
    const copy = current.options.editorProps.clipboardTextSerializer?.(slice, current.view);
    expect(copy).toContain('[@Chetan](mention:user)');
    expect(copy).not.toContain('<span');
    wrapper.unmount();
  });

  it('pastes Markdown rather than raw HTML clipboard markup', async () => {
    const wrapper = await render();
    const current = instance(wrapper);
    const event = new Event('paste', { bubbles: true, cancelable: true });
    Object.defineProperty(event, 'clipboardData', {
      value: {
        getData: (type: string) =>
          type === 'text/plain'
            ? '**Pasted**'
            : '<img src="https://evil.test/pixel" onerror="alert(1)">',
      },
    });
    current.view.dom.dispatchEvent(event);
    await flushPromises();
    expect(wrapper.get('[role="textbox"] strong').text()).toBe('Pasted');
    expect(wrapper.find('img').exists()).toBe(false);
    expect(wrapper.props('modelValue')).toBe('**Pasted**');
    wrapper.unmount();
  });
});
