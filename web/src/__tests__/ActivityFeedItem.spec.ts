import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';

import ActivityFeedItem from '../components/ActivityFeedItem.vue';
import { useActivitiesStore } from '../stores/activities';
import {
  type FeedEvent,
  FeedEventSourceTypeFileUploadBatch,
  FeedStateUnread,
} from '../types/models';

function buildUploadEvent(overrides: Partial<FeedEvent> = {}): FeedEvent {
  return {
    id: 'evt_1',
    user_id: 'user_1',
    event_type: 'file_uploaded',
    source_type: 'file_activity',
    source_id: 'activity_1',
    file_path: 'Acme/reports/report.pdf',
    created_at: '2026-03-11T11:30:00Z',
    is_read: false,
    ...overrides,
  };
}

describe('ActivityFeedItem', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    const activitiesStore = useActivitiesStore();
    activitiesStore.currentState = FeedStateUnread;
  });

  it('links uploaded notifications to the file route using file_id', () => {
    const wrapper = mount(ActivityFeedItem, {
      props: {
        item: buildUploadEvent({
          file_id: 'file_1',
          actor_name: 'Admin User',
          client_name: 'Acme',
        }),
      },
      global: {
        stubs: {
          Button: {
            template: '<button><slot /></button>',
          },
        },
      },
    });

    expect(wrapper.find('a[href="/files/file_1"]').exists()).toBe(true);
    expect(wrapper.find('a[href="/files/activity_1"]').exists()).toBe(false);
  });

  it('leaves uploaded notifications as plain text when file_id is missing', () => {
    const wrapper = mount(ActivityFeedItem, {
      props: {
        item: buildUploadEvent({
          actor_name: 'Admin User',
          client_name: 'Acme',
        }),
      },
      global: {
        stubs: {
          Button: {
            template: '<button><slot /></button>',
          },
        },
      },
    });

    expect(wrapper.find('a').exists()).toBe(false);
    expect(wrapper.text()).toContain('report.pdf');
  });

  it('renders grouped upload notifications as one summary with multiple file links', () => {
    const wrapper = mount(ActivityFeedItem, {
      props: {
        item: buildUploadEvent({
          source_type: FeedEventSourceTypeFileUploadBatch,
          source_id: 'batch_1',
          file_id: 'file_1',
          file_path: 'Acme/reports/report.pdf',
          file_count: 2,
          files: [
            { id: 'file_1', path: 'Acme/reports/report.pdf' },
            { id: 'file_2', path: 'Acme/reports/invoice.pdf' },
          ],
          actor_name: 'Admin User',
          client_name: 'Acme',
        }),
      },
      global: {
        stubs: {
          Button: {
            template: '<button><slot /></button>',
          },
        },
      },
    });

    expect(wrapper.text()).toContain('2 files');
    expect(wrapper.find('a[href="/files/file_1"]').exists()).toBe(true);
    expect(wrapper.find('a[href="/files/file_2"]').exists()).toBe(true);
    expect(wrapper.text()).toContain('report.pdf');
    expect(wrapper.text()).toContain('invoice.pdf');
  });
});
