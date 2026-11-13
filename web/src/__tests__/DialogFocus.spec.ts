import { flushPromises, mount } from '@vue/test-utils';
import OpenVue from 'openvue/config';
import Dialog from 'openvue/dialog';
import { afterEach, describe, expect, it, vi } from 'vitest';

// Regression coverage for patches/openvue@1.1.1.patch, not the general Dialog API.
describe('patched OpenVue dialog behavior', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it.each([
    { maximizable: true, section: 'pcMaximizeButton' },
    { maximizable: false, section: 'pcCloseButton' },
  ])('focuses $section without forcing a ring', async ({ maximizable, section }) => {
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

  it('dismisses only the top dialog on Escape or an outside click', async () => {
    const bottom = mount(Dialog, {
      props: { visible: true, modal: true, dismissableMask: true, header: 'Sharing' },
      global: { plugins: [OpenVue], stubs: { transition: false } },
      attachTo: document.body,
    });
    const top = mount(Dialog, {
      props: { visible: true, modal: true, dismissableMask: true, header: 'Discard changes?' },
      global: { plugins: [OpenVue], stubs: { transition: false } },
      attachTo: document.body,
    });
    try {
      await flushPromises();
      document.dispatchEvent(
        new KeyboardEvent('keydown', {
          key: 'Escape',
          code: 'Escape',
          bubbles: true,
          cancelable: true,
        }),
      );
      expect(top.emitted('update:visible')).toEqual([[false]]);
      expect(bottom.emitted('update:visible')).toBeUndefined();
      const masks = document.querySelectorAll('.p-dialog-mask');
      const mask = masks[masks.length - 1];
      mask.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));
      mask.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }));
      expect(top.emitted('update:visible')).toHaveLength(2);
      expect(bottom.emitted('update:visible')).toBeUndefined();
    } finally {
      top.unmount();
      bottom.unmount();
    }
  });
});
