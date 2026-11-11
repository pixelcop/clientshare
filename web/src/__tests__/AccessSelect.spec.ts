import { mount } from '@vue/test-utils';
import Select from 'openvue/select';
import { describe, expect, it, vi } from 'vitest';

import AccessSelect from '@/components/AccessSelect.vue';

function control(options: object[], attrs = {}, isFile = false) {
  return mount(AccessSelect, {
    props: { isFile },
    attrs: { options, optionLabel: 'name', optionValue: 'id', ...attrs },
    global: {
      stubs: {
        Select: {
          props: {
            options: Array,
            modelValue: String,
            ariaLabel: String,
            disabled: Boolean,
            overlayClass: String,
            scrollHeight: String,
            checkmark: Boolean,
          },
          template: '<div><slot v-for="option in options" name="option" :option="option" /></div>',
        },
      },
    },
  });
}

describe('expanded access selector', () => {
  it('uses one expanded, scrollable overlay and forwards control attributes and updates', () => {
    const update = vi.fn();
    const wrapper = control([{ name: 'Viewer', id: 'viewer' }], {
      modelValue: 'viewer',
      ariaLabel: 'Workspace role',
      disabled: true,
      'onUpdate:modelValue': update,
    });
    const select = wrapper.getComponent(Select);
    expect(select.props()).toMatchObject({
      overlayClass: 'access-select-overlay',
      scrollHeight: '24rem',
      checkmark: true,
      modelValue: 'viewer',
      ariaLabel: 'Workspace role',
      disabled: true,
    });
    select.vm.$emit('update:modelValue', 'editor');
    expect(update).toHaveBeenCalledWith('editor');
    wrapper.unmount();
  });

  it('shows actual role permissions instead of an editable role description', async () => {
    const wrapper = control([
      {
        name: 'Custom role',
        id: 'custom',
        description: 'Misleading description',
        can_read: true,
        can_upload: true,
        can_delete: false,
        can_manage_access: true,
      },
    ]);
    expect(wrapper.get('strong').text()).toBe('Custom role');
    expect(wrapper.get('small').text()).toBe('View and download · Upload · Manage access');
    await wrapper.setProps({ isFile: true });
    expect(wrapper.get('small').text()).toBe('View and download · Manage access');
    wrapper.unmount();
  });

  it('supports functional labels and descriptive identity and link choices', () => {
    const wrapper = control([{ name: 'Archived', description: 'View only' }], {
      optionLabel: (option: { name: string }) => `${option.name} (archived)`,
    });
    expect(wrapper.get('strong').text()).toBe('Archived (archived)');
    expect(wrapper.get('small').text()).toBe('View only');
    wrapper.unmount();
  });
});
