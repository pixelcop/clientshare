import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';

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
          },
          template: '<div><slot v-for="option in options" name="option" :option="option" /></div>',
        },
      },
    },
  });
}

describe('expanded access selector', () => {
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
