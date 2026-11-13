import { flushPromises, mount } from '@vue/test-utils';
import OpenVue from 'openvue/config';
import { describe, expect, it, vi } from 'vitest';

import ImagePreview from '@/components/ImagePreview.vue';
import { imageClickZoom, imageGeometry, stepImageZoom } from '@/utils/imagePreview';

vi.mock('@vueuse/core', () => ({ useResizeObserver: vi.fn() }));

describe('image zoom geometry', () => {
  it('starts smaller images at 100% and increases until the image exceeds the viewport, then resets', () => {
    const geometry = imageGeometry(400, 300, 1000, 800, 0);
    expect(geometry.fitScale).toBe(1);
    let scale = 1;
    const levels = [scale];
    while (scale * 400 <= 1000 && scale * 300 <= 800) {
      scale = imageClickZoom(scale, geometry);
      levels.push(scale);
    }
    expect(levels).toEqual([1, 1.25, 1.5, 2, 3]);
    expect(imageClickZoom(scale, geometry)).toBe(1);
  });

  it('toggles oversized images between fit and 100%, even after intermediate control zooms', () => {
    const geometry = imageGeometry(2000, 1000, 1000, 800, 0);
    expect(geometry.fitScale).toBe(0.5);
    expect(imageClickZoom(0.5, geometry)).toBe(1);
    expect(imageClickZoom(0.75, geometry)).toBe(1);
    expect(imageClickZoom(1, geometry)).toBe(0.5);
    expect(imageClickZoom(2, geometry)).toBe(0.5);
  });

  it('accounts for rotated bounds without changing the source dimensions', () => {
    expect(imageGeometry(2000, 1000, 1000, 800, 1)).toMatchObject({ width: 1000, height: 2000, fitScale: 0.4 });
    expect(imageGeometry(2000, 1000, 1000, 800, 2)).toMatchObject({ width: 2000, height: 1000, fitScale: 0.5 });
  });

  it('includes exact fit as a zoom stop and bounds the buttons for huge and tiny images', () => {
    const huge = imageGeometry(100000, 50000, 1000, 800, 0);
    expect(stepImageZoom(huge.fitScale, -1, huge)).toBe(huge.minScale);
    expect(stepImageZoom(8, 1, huge)).toBe(8);
    const tiny = imageGeometry(8, 8, 1000, 800, 0);
    expect(tiny.maxScale * 8).toBeGreaterThan(800);
    expect(imageClickZoom(tiny.maxScale, tiny)).toBe(1);
    const fitted = imageGeometry(3000, 2000, 1000, 800, 0);
    expect(stepImageZoom(0.5, -1, fitted)).toBe(fitted.fitScale);
  });
});

async function render(width = 400, height = 300) {
  const wrapper = mount(ImagePreview, { props: { fileUrl: '/api/files/image/view', fileName: 'photo.png' }, global: { plugins: [OpenVue] } });
  const stage = wrapper.get('.image-stage').element;
  Object.defineProperties(stage, { clientWidth: { value: 1000 }, clientHeight: { value: 800 } });
  const image = wrapper.get('img');
  Object.defineProperties(image.element, { naturalWidth: { value: width }, naturalHeight: { value: height } });
  await image.trigger('load');
  await flushPromises();
  return wrapper;
}

describe('image preview controls', () => {
  it('keeps controls hidden until load and forwards loading errors', async () => {
    const wrapper = mount(ImagePreview, { props: { fileUrl: '/view', fileName: 'photo.png' } });
    expect(wrapper.find('[aria-label="Image controls"]').exists()).toBe(false);
    expect(wrapper.get('[role="status"]').text()).toBe('Loading image…');
    await wrapper.get('img').trigger('error');
    expect(wrapper.emitted('error')).toEqual([[]]);
    wrapper.unmount();
  });

  it('zooms with controls and resets on double-click or keyboard activation of the percentage', async () => {
    const wrapper = await render();
    expect(wrapper.get('.image-zoom-value').text()).toBe('100%');
    await wrapper.get('[aria-label="Zoom in"]').trigger('click');
    expect(wrapper.get('.image-zoom-value').text()).toBe('125%');
    await wrapper.get('.image-zoom-value').trigger('dblclick');
    expect(wrapper.get('.image-zoom-value').text()).toBe('100%');
    await wrapper.get('[aria-label="Zoom out"]').trigger('click');
    expect(wrapper.get('.image-zoom-value').text()).toBe('75%');
    await wrapper.get('.image-zoom-value').trigger('keydown', { key: 'Enter' });
    expect(wrapper.get('.image-zoom-value').text()).toBe('100%');
    wrapper.unmount();
  });

  it('toggles a large image between fit and 100% with the matching reset action', async () => {
    const wrapper = await render(2000, 1000);
    expect(wrapper.get('.image-zoom-value').text()).toBe('50%');
    await wrapper.get('.image-target').trigger('click');
    expect(wrapper.get('.image-zoom-value').text()).toBe('100%');
    expect(wrapper.get('.image-target').classes()).toContain('will-reset');
    await wrapper.get('.image-target').trigger('click');
    expect(wrapper.get('.image-zoom-value').text()).toBe('50%');
    expect(wrapper.get('.image-target').classes()).not.toContain('will-reset');
    wrapper.unmount();
  });

  it('rotates left through all four orientations without changing the source or sending mutations', async () => {
    const wrapper = await render(2000, 1000);
    await wrapper.get('[aria-label="Rotate left"]').trigger('click');
    expect(wrapper.get('img').attributes('style')).toContain('rotate(-90deg)');
    expect(wrapper.get('.image-zoom-value').text()).toBe('40%');
    await wrapper.get('[aria-label="Rotate left"]').trigger('click');
    await wrapper.get('[aria-label="Rotate left"]').trigger('click');
    await wrapper.get('[aria-label="Rotate left"]').trigger('click');
    expect(wrapper.get('img').attributes('style')).toContain('rotate(0deg)');
    expect(wrapper.get('img').attributes('src')).toBe('/api/files/image/view');
    expect(wrapper.emitted('error')).toBeUndefined();
    wrapper.unmount();
  });

  it('resets zoom and rotation when another image loads', async () => {
    const wrapper = await render();
    await wrapper.get('[aria-label="Zoom in"]').trigger('click');
    await wrapper.get('[aria-label="Rotate left"]').trigger('click');
    await wrapper.setProps({ fileUrl: '/api/files/another/view' });
    expect(wrapper.find('[aria-label="Image controls"]').exists()).toBe(false);
    Object.defineProperties(wrapper.get('img').element, { naturalWidth: { value: 100 }, naturalHeight: { value: 100 } });
    await wrapper.get('img').trigger('load');
    expect(wrapper.get('.image-zoom-value').text()).toBe('100%');
    expect(wrapper.get('img').attributes('style')).toContain('rotate(0deg)');
    wrapper.unmount();
  });
});
