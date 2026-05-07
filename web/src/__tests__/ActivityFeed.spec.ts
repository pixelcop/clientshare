import { mount } from '@vue/test-utils';
import axios from 'axios';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useAuthStore } from '../stores/auth';
import ActivityFeed from '../views/ActivityFeed.vue';

vi.mock('axios', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(),
    interceptors: {
      response: {
        use: vi.fn(),
      },
    },
    defaults: {
      headers: {
        common: {},
      },
    },
  },
}));

const mockedAxios = axios as unknown as {
  get: ReturnType<typeof vi.fn>;
  post: ReturnType<typeof vi.fn>;
};

function flushPromises() {
  return new Promise<void>((resolve) => {
    setTimeout(resolve, 0);
  });
}

describe('ActivityFeed', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    mockedAxios.get.mockReset();
    mockedAxios.post.mockReset();
  });

  it('renders upload and invite events for admin users', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        summary: {
          unread_count: 2,
          read_count: 3,
        },
        pagination: {
          total_items: 2,
          total_pages: 1,
          page: 1,
          page_size: 20,
          has_more: false,
          items_per_page: 2,
        },
        items: [
          {
            id: 'evt_1',
            event_type: 'invite_accepted',
            source_type: 'invite_token',
            source_id: 'invite_1',
            file_path: '',
            created_at: '2026-03-11T12:00:00Z',
            subject_name: 'Invited User',
            subject_email: 'invite@test.local',
            is_read: false,
          },
          {
            id: 'evt_2',
            event_type: 'file_uploaded',
            client_id: 'client_1',
            actor_user_id: 'user_1',
            source_type: 'file_upload_batch',
            source_id: 'batch_1',
            file_id: 'file_1',
            file_path: 'Acme/reports/report.pdf',
            file_count: 2,
            files: [
              {
                id: 'file_1',
                path: 'Acme/reports/report.pdf',
              },
              {
                id: 'file_2',
                path: 'Acme/reports/invoice.pdf',
              },
            ],
            created_at: '2026-03-11T11:30:00Z',
            client_name: 'Acme',
            actor_name: 'Admin User',
            actor_email: 'admin@test.local',
            is_read: false,
          },
        ],
      },
    });

    const authStore = useAuthStore();
    authStore.user = {
      id: 'admin_1',
      email: 'admin@test.local',
      role: 'admin',
      name: 'Admin User',
      invite_accepted: true,
      created_at: '2026-03-11T10:00:00Z',
      updated_at: '2026-03-11T10:00:00Z',
    };

    const wrapper = mount(ActivityFeed, {
      global: {
        stubs: {
          Card: {
            template:
              '<div><slot name="header" /><slot name="content" /><slot name="footer" /></div>',
          },
          Button: {
            template: '<button><slot /></button>',
          },
          Paginator: true,
        },
      },
    });

    await flushPromises();

    expect(mockedAxios.get).toHaveBeenCalledWith('/api/feed', {
      params: { page: 1, page_size: 20, state: 'unread' },
    });
    expect(wrapper.text()).toContain('Activity');
    expect(wrapper.text()).toContain('Unread');
    expect(wrapper.text()).toContain('Read');
    expect(wrapper.text()).toContain('Clear all');
    expect(wrapper.text()).toContain('Invited User');
    expect(wrapper.text()).toContain('accepted invite');
    expect(wrapper.text()).toContain('2 files');
    expect(wrapper.text()).toContain('report.pdf');
    expect(wrapper.text()).toContain('invoice.pdf');
    expect(wrapper.text()).toContain('uploaded by Admin User');
    expect(wrapper.text()).toContain('Acme');
  });

  it('clears unread notifications from the toolbar action', async () => {
    mockedAxios.get
      .mockResolvedValueOnce({
        data: {
          summary: {
            unread_count: 1,
            read_count: 0,
          },
          pagination: {
            total_items: 1,
            total_pages: 1,
            page: 1,
            page_size: 20,
            has_more: false,
            items_per_page: 1,
          },
          items: [
            {
              id: 'evt_1',
              event_type: 'file_uploaded',
              source_type: 'file_activity',
              source_id: 'activity_1',
              file_id: 'file_1',
              file_path: 'Acme/reports/report.pdf',
              created_at: '2026-03-11T11:30:00Z',
              client_name: 'Acme',
              actor_name: 'Admin User',
              actor_email: 'admin@test.local',
              is_read: false,
            },
          ],
        },
      })
      .mockResolvedValueOnce({
        data: {
          summary: {
            unread_count: 0,
            read_count: 1,
          },
          pagination: {
            total_items: 0,
            total_pages: 0,
            page: 1,
            page_size: 20,
            has_more: false,
            items_per_page: 0,
          },
          items: [],
        },
      });
    mockedAxios.post.mockResolvedValue({ data: { status: 'ok' } });

    const authStore = useAuthStore();
    authStore.user = {
      id: 'admin_1',
      email: 'admin@test.local',
      role: 'admin',
      name: 'Admin User',
      invite_accepted: true,
      created_at: '2026-03-11T10:00:00Z',
      updated_at: '2026-03-11T10:00:00Z',
    };

    const wrapper = mount(ActivityFeed, {
      global: {
        stubs: {
          Card: {
            template:
              '<div><slot name="header" /><slot name="content" /><slot name="footer" /></div>',
          },
          Paginator: true,
        },
      },
    });

    await flushPromises();

    const clearButton = wrapper.get('button.clear-all');
    expect(clearButton.attributes('disabled')).toBeUndefined();

    await clearButton.trigger('click');
    await flushPromises();

    expect(mockedAxios.post).toHaveBeenCalledWith('/api/feed/read-all');
    expect(mockedAxios.get).toHaveBeenNthCalledWith(2, '/api/feed', {
      params: { page: 1, page_size: 20, state: 'unread' },
    });
    expect(wrapper.text()).toContain('No unread activity.');
  });
});
