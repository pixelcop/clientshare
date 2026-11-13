import { flushPromises, mount } from '@vue/test-utils';
import OpenVue from 'openvue/config';
import { afterEach, describe, expect, it, vi } from 'vitest';

import FilePreview from '@/components/FilePreview.vue';

vi.mock('@embedpdf/vue-pdf-viewer', () => ({ PDFViewer: { template: '<div />' }, ZoomMode: { Automatic: 'automatic' } }));

describe('file preview', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('shows a download fallback for a failed text response instead of rendering an HTTP error body', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, text: async () => 'private server error' }));
    const wrapper = mount(FilePreview, { props: { fileUrl: '/content', downloadUrl: '/download', fileType: 'text/plain', fileName: 'report.txt' }, global: { plugins: [OpenVue] } });
    await flushPromises();
    expect(wrapper.text()).toContain('Preview couldn’t be loaded');
    expect(wrapper.text()).not.toContain('private server error');
    expect(wrapper.get('a').attributes('download')).toBe('report.txt');
    wrapper.unmount();
  });

  it('renders text literally, including markup, after a successful response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, text: async () => '<script>alert(1)</script>' }));
    const wrapper = mount(FilePreview, { props: { fileUrl: '/content', downloadUrl: '/download', fileType: 'text/plain', maximized: true } });
    await flushPromises();
    expect(wrapper.get('code').text()).toBe('<script>alert(1)</script>');
    expect(wrapper.find('script').exists()).toBe(false);
    wrapper.unmount();
  });

  it('offers a real download link when an image cannot load', async () => {
    const wrapper = mount(FilePreview, { props: { fileUrl: '/content', downloadUrl: '/download', fileType: 'image/png', fileName: 'image.png' }, global: { plugins: [OpenVue] } });
    expect(wrapper.get('img').attributes('src')).toBe('/content');
    await wrapper.get('img').trigger('error');
    expect(wrapper.text()).toContain('Preview couldn’t be loaded');
    expect(wrapper.get('a').attributes('href')).toBe('/download');
    wrapper.unmount();
  });
});
