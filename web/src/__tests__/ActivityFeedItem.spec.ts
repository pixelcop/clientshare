import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

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

const RouterLink = {
  props: ['to'],
  template: "<a :href=\"typeof to === 'string' ? to : '#'\" @click.prevent><slot /></a>",
};

function renderItem(overrides: Partial<FeedEvent>) {
  return mount(ActivityFeedItem, {
    props: { item: buildUploadEvent(overrides) },
    global: { stubs: { RouterLink, Button: true } },
  });
}

describe('ActivityFeedItem', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    const activitiesStore = useActivitiesStore();
    activitiesStore.currentState = FeedStateUnread;
  });

  it('opens the uploaded file and marks its notification as read', async () => {
    const markRead = vi.spyOn(useActivitiesStore(), 'markRead').mockResolvedValue(undefined);
    const wrapper = renderItem({ file_id: 'file_1' });

    await wrapper.get('a[href="/files/file_1"]').trigger('click');
    expect(markRead).toHaveBeenCalledExactlyOnceWith('evt_1');
  });

  it('leaves uploaded notifications as plain text when file_id is missing', () => {
    const wrapper = renderItem({});

    expect(wrapper.find('a').exists()).toBe(false);
    expect(wrapper.text()).toContain('report.pdf');
  });

  it('marks the grouped upload as read when one of its files is opened', async () => {
    const markRead = vi.spyOn(useActivitiesStore(), 'markRead').mockResolvedValue(undefined);
    const wrapper = renderItem({
      source_type: FeedEventSourceTypeFileUploadBatch,
      source_id: 'batch_1',
      file_count: 2,
      files: [
        { id: 'file_1', path: 'Acme/reports/report.pdf' },
        { id: 'file_2', path: 'Acme/reports/invoice.pdf' },
      ],
    });

    expect(wrapper.text()).toContain('2 files');
    expect(wrapper.find('a[href="/files/file_1"]').exists()).toBe(true);
    await wrapper.get('a[href="/files/file_2"]').trigger('click');
    expect(markRead).toHaveBeenCalledExactlyOnceWith('evt_1');
    expect(wrapper.text()).toContain('report.pdf');
    expect(wrapper.text()).toContain('invoice.pdf');
  });

  it.each([
    { event_type: 'file_comment', file_id: 'file_1' },
    {
      event_type: 'request_created',
      request_open: true,
      request_id: 'request_1',
      request_title: 'October close',
    },
  ])('marks the activity as read when opening its link (%o)', async (overrides) => {
    const markRead = vi.spyOn(useActivitiesStore(), 'markRead').mockResolvedValue(undefined);
    const wrapper = renderItem(overrides);

    await wrapper.get('a').trigger('click');
    expect(markRead).toHaveBeenCalledExactlyOnceWith('evt_1');
  });

  it('does not mark an already-read item again', async () => {
    const markRead = vi.spyOn(useActivitiesStore(), 'markRead').mockResolvedValue(undefined);
    const wrapper = renderItem({ file_id: 'file_1', is_read: true });
    await wrapper.get('a').trigger('click');
    expect(markRead).not.toHaveBeenCalled();
  });
});
