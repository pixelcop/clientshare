import { mount } from '@vue/test-utils';
import Button from 'openvue/button';
import DatePicker from 'openvue/datepicker';
import Dialog from 'openvue/dialog';
import Select from 'openvue/select';
import { describe, expect, it } from 'vitest';

import SharingExpiry from '@/components/SharingExpiry.vue';

function control(modelValue: Date | null = null, required = false) {
  return mount(SharingExpiry, {
    props: { modelValue, required },
    global: {
      stubs: {
        Select: {
          props: ['modelValue', 'options', 'ariaLabel', 'pt'],
          template: '<div><slot name="value" /></div>',
        },
        DatePicker: { props: ['modelValue'], template: '<div />' },
        Button: { props: ['label', 'disabled'], template: '<button />' },
        Dialog: { props: ['visible'], template: '<div><slot /><slot name="footer" /></div>' },
      },
    },
  });
}

describe('sharing expiry', () => {
  it('highlights only a set expiry and exposes its exact date without a native title', async () => {
    const wrapper = control();
    expect(wrapper.find('.expiry-clock.is-active').exists()).toBe(false);
    expect(wrapper.getComponent(Select).attributes('title')).toBeUndefined();
    await wrapper.setProps({ modelValue: new Date(2027, 2, 14, 17, 30) });
    expect(wrapper.find('.expiry-clock.is-active').exists()).toBe(true);
    expect(wrapper.getComponent(Select).attributes('title')).toBeUndefined();
    expect(wrapper.getComponent(Select).props('pt').label['aria-description']).toContain(
      'Access expires on',
    );
    wrapper.unmount();
  });
  it('offers only no expiry, 7 days, 30 days and custom, with no unlimited links', () => {
    const optional = control();
    expect(
      optional
        .getComponent(Select)
        .props('options')
        .map((option: { value: string }) => option.value),
    ).toEqual(['none', '7', '30', 'custom']);
    const required = control(null, true);
    expect(
      required
        .getComponent(Select)
        .props('options')
        .map((option: { value: string }) => option.value),
    ).toEqual(['7', '30', 'custom']);
    optional.unmount();
    required.unmount();
  });

  it('recognizes an existing 30-day preset without changing its timestamp', () => {
    const date = new Date(Date.now() + 30 * 86400000);
    const wrapper = control(date);
    expect(wrapper.getComponent(Select).props('modelValue')).toBe('30');
    expect(wrapper.getComponent(Select).props('ariaLabel')).toBe('Expiry: Expires in 30 days');
    expect(wrapper.emitted('update:modelValue')).toBeUndefined();
    wrapper.unmount();
  });

  it('does not change expiry when the custom calendar is cancelled', async () => {
    const wrapper = control();
    wrapper.getComponent(Select).vm.$emit('update:modelValue', 'custom');
    await wrapper.vm.$nextTick();
    expect(wrapper.getComponent(Dialog).props('visible')).toBe(true);
    wrapper
      .findAllComponents(Button)
      .find((button) => button.props('label') === 'Cancel')!
      .vm.$emit('click');
    await wrapper.vm.$nextTick();
    expect(wrapper.getComponent(Dialog).props('visible')).toBe(false);
    expect(wrapper.emitted('update:modelValue')).toBeUndefined();
    wrapper.unmount();
  });

  it('applies custom expiry at the end of the selected local day', async () => {
    const wrapper = control();
    wrapper.getComponent(Select).vm.$emit('update:modelValue', 'custom');
    const date = new Date(2027, 2, 14);
    wrapper.getComponent(DatePicker).vm.$emit('update:modelValue', date);
    await wrapper.vm.$nextTick();
    wrapper
      .findAllComponents(Button)
      .find((button) => button.props('label') === 'Set expiry')!
      .vm.$emit('click');
    const expected = new Date(date);
    expected.setHours(23, 59, 59, 999);
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([expected]);
    wrapper.unmount();
  });
});
