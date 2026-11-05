import { flushPromises, mount } from '@vue/test-utils';
import OpenVue from 'openvue/config';
import Dialog from 'openvue/dialog';
import { afterEach, describe, expect, it, vi } from 'vitest';

describe('OpenVue dialog initial focus', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it.each([
    { maximizable: true, autofocus: false, section: 'pcMaximizeButton' },
    { maximizable: false, autofocus: false, section: 'pcCloseButton' },
    { maximizable: true, autofocus: true, section: 'input' },
  ])('focuses $section without forcing a ring', async ({ maximizable, autofocus, section }) => {
    const focus = vi.spyOn(HTMLElement.prototype, 'focus');
    const wrapper = mount(Dialog, {
      props: {
        visible: true,
        modal: true,
        maximizable,
        header: 'Preview',
        maximizeButtonProps: { 'data-testid': 'pcMaximizeButton' },
        closeButtonProps: { 'data-testid': 'pcCloseButton' },
      },
      slots: autofocus ? { default: '<input autofocus data-testid="input" />' } : {},
      global: { plugins: [OpenVue], stubs: { transition: false } },
      attachTo: document.body,
    });

    try {
      await flushPromises();
      const target = document.querySelector(`[data-testid="${section}"]`);
      expect(target).not.toBeNull();
      await vi.waitFor(() => expect(document.activeElement).toBe(target));
      expect(focus.mock.calls.length).toBeGreaterThan(0);
      expect(focus.mock.calls.every(([options]) => !options || !('focusVisible' in options))).toBe(
        true,
      );
    } finally {
      wrapper.unmount();
    }
  });
});
