<script setup lang="ts">
import { Button, Panel } from 'openvue';

import type { SpotlightRect } from '@/composables/collectionSpotlight';
import { placeCollectionCoach } from '@/composables/collectionSpotlight';

const props = defineProps<{ request: { targets: string[]; title: string; body: string } | null }>();
const emit = defineEmits<{
  close: [];
  unavailable: [];
  obstructed: [position: 'top' | 'bottom'];
}>();
const rect = ref<SpotlightRect | null>(null);
const coachHeight = ref(164);
const viewport = ref({ width: window.innerWidth, height: window.innerHeight, top: 0, left: 0 });
const portal = ref<HTMLElement | string>('body');
const origin = ref({ left: 0, top: 0 });
const coach = ref<InstanceType<typeof Panel>>();
const placement = computed(
  () => rect.value && placeCollectionCoach(rect.value, viewport.value, coachHeight.value),
);
const dimPath = computed(() => {
  const r = rect.value;
  if (!r) {
    return '';
  }
  const pad = 6;
  return `M0 0H${window.innerWidth}V${window.innerHeight}H0Z M${r.left - pad} ${r.top - pad}V${r.top + r.height + pad}H${r.left + r.width + pad}V${r.top - pad}Z`;
});
const arrow = computed(() => {
  const p = placement.value,
    r = rect.value;
  if (!p || !r) {
    return '';
  }
  const cx = r.left + r.width / 2,
    cy = r.top + r.height / 2;
  if (p.side === 'left') {
    return `${p.x + p.width + 2},${Math.min(p.y + 36, cy)} ${r.left - 10},${cy}`;
  }
  if (p.side === 'right') {
    return `${p.x - 2},${Math.min(p.y + 36, cy)} ${r.left + r.width + 10},${cy}`;
  }
  const x = Math.max(p.x + 20, Math.min(cx, p.x + p.width - 20));
  return p.side === 'above'
    ? `${x},${p.y + coachHeight.value + 2} ${cx},${r.top - 10}`
    : `${x},${p.y - 2} ${cx},${r.top + r.height + 10}`;
});
let target: HTMLElement | null = null;
let previous: HTMLElement | null = null;
let describedBy: string | null = null;
let observer: MutationObserver | undefined;
let resize: ResizeObserver | undefined;
let deadline: ReturnType<typeof setTimeout> | undefined;
let frame = 0;
let version = 0;

function cleanup() {
  version += 1;
  observer?.disconnect();
  resize?.disconnect();
  clearTimeout(deadline);
  cancelAnimationFrame(frame);
  if (target) {
    if (describedBy === null) {
      target.removeAttribute('aria-describedby');
    } else {
      target.setAttribute('aria-describedby', describedBy);
    }
  }
  target = null;
  rect.value = null;
  previous = null;
}
function close(restore = true) {
  const returnTo = restore ? previous : null;
  cleanup();
  emit('close');
  // On mobile the step is hidden until the temporary header-only panel expands.
  void nextTick(() => {
    if (returnTo?.isConnected && !props.request) {
      returnTo.focus({ preventScroll: true });
    }
  });
}
function measure() {
  if (!target) {
    return;
  }
  if (!target.isConnected || !target.getClientRects().length) {
    close(false);
    return;
  }
  const r = target.getBoundingClientRect();
  const dock = document.querySelector<HTMLElement>('.collection-dock');
  if (dock && !target.closest('[role="dialog"]')) {
    const b = dock.getBoundingClientRect();
    if (r.left < b.right && r.right > b.left && r.top < b.bottom && r.bottom > b.top) {
      emit('obstructed', r.top > window.innerHeight / 2 ? 'top' : 'bottom');
    }
  }
  // Dialog's transform establishes the containing block for its fixed descendants.
  const host = typeof portal.value === 'string' ? null : portal.value;
  const bounds =
    host && getComputedStyle(host).transform !== 'none' ? host.getBoundingClientRect() : null;
  origin.value = {
    left: bounds ? bounds.left + host!.clientLeft : 0,
    top: bounds ? bounds.top + host!.clientTop : 0,
  };
  if (coach.value?.$el) {
    coachHeight.value = (coach.value.$el as HTMLElement).getBoundingClientRect().height;
  }
  const vv = window.visualViewport;
  viewport.value = {
    width: vv?.width || window.innerWidth,
    height: vv?.height || window.innerHeight,
    top: vv?.offsetTop || 0,
    left: vv?.offsetLeft || 0,
  };
  const { top, left, width, height } = viewport.value;
  if (r.bottom < top || r.top > top + height || r.right < left || r.left > left + width) {
    close(false);
    return;
  }
  rect.value = { left: r.left, top: r.top, width: r.width, height: r.height };
}
function scheduleMeasure() {
  cancelAnimationFrame(frame);
  frame = requestAnimationFrame(measure);
}
function escape(event: KeyboardEvent) {
  if (props.request && event.key === 'Escape') {
    event.preventDefault();
    event.stopImmediatePropagation();
    close();
  } else if (
    target?.contains(event.target as Node) &&
    !['Tab', 'Shift', 'Control', 'Alt', 'Meta'].includes(event.key)
  ) {
    close(false);
  }
}
function pointer(event: PointerEvent) {
  if (!rect.value || (event.target as HTMLElement).closest('[data-collection-coach]')) {
    return;
  }
  // Clicking the real control continues the real action; the spotlight never intercepts it.
  close(false);
}
function input(event: Event) {
  if (target?.contains(event.target as Node)) {
    close(false);
  }
}
watch(
  () => props.request,
  async (request) => {
    cleanup();
    if (!request) {
      return;
    }
    const current = version;
    previous = document.activeElement as HTMLElement;
    await nextTick();
    if (current !== version) {
      return;
    }
    function findTarget() {
      if (current !== version || target) {
        return;
      }
      const element = request!.targets
        .map((id) => document.getElementById(id))
        .find((el) => el?.getClientRects().length);
      if (!element) {
        return;
      }
      const dialog = Array.from(
        document.querySelectorAll<HTMLElement>('[role="dialog"][aria-modal="true"]'),
      ).find((el) => el.getClientRects().length);
      if (dialog && !dialog.contains(element)) {
        return;
      }
      observer?.disconnect();
      clearTimeout(deadline);
      target = element;
      // Keep the note inside an existing dialog's focus scope when annotating a form.
      portal.value = element.closest<HTMLElement>('[role="dialog"]') || 'body';
      describedBy = element.getAttribute('aria-describedby');
      element.setAttribute(
        'aria-describedby',
        [describedBy, 'collection-coach-description'].filter(Boolean).join(' '),
      );
      element.scrollIntoView({ block: 'center', inline: 'nearest', behavior: 'instant' });
      element.focus({ preventScroll: true });
      measure();
      resize = new ResizeObserver(scheduleMeasure);
      resize.observe(element);
      // Dialog enter transitions can move a control without changing its dimensions.
      deadline = setTimeout(scheduleMeasure, 240);
    }
    findTarget();
    if (!target) {
      observer = new MutationObserver(findTarget);
      observer.observe(document.body, {
        childList: true,
        subtree: true,
        attributes: true,
        attributeFilter: ['class', 'style'],
      });
      deadline = setTimeout(() => {
        cleanup();
        emit('unavailable');
      }, 1200);
    }
  },
  { immediate: true },
);
watch(coach, (panel) => {
  if (!panel?.$el) {
    return;
  }
  const element = panel.$el as HTMLElement;
  coachHeight.value = element.getBoundingClientRect().height;
  resize?.observe(element);
});
onMounted(() => {
  document.addEventListener('keydown', escape, true);
  document.addEventListener('pointerdown', pointer, true);
  document.addEventListener('input', input, true);
  document.addEventListener('scroll', scheduleMeasure, true);
  window.addEventListener('resize', scheduleMeasure);
  window.visualViewport?.addEventListener('resize', scheduleMeasure);
});
onBeforeUnmount(() => {
  cleanup();
  document.removeEventListener('keydown', escape, true);
  document.removeEventListener('pointerdown', pointer, true);
  document.removeEventListener('input', input, true);
  document.removeEventListener('scroll', scheduleMeasure, true);
  window.removeEventListener('resize', scheduleMeasure);
  window.visualViewport?.removeEventListener('resize', scheduleMeasure);
});
</script>

<template>
  <Teleport :to="portal">
    <svg
      v-if="rect && request"
      class="collection-spotlight collection-theme"
      :style="{
        left: `${-origin.left}px`,
        top: `${-origin.top}px`,
        width: `${viewport.width}px`,
        height: `${viewport.height}px`,
      }"
      aria-hidden="true"
    >
      <defs>
        <marker
          id="collection-arrow"
          markerWidth="6"
          markerHeight="6"
          refX="5"
          refY="3"
          orient="auto"
        >
          <path d="M0 0L6 3L0 6" fill="none" stroke="var(--collection-marker)" stroke-width="1.5" />
        </marker>
      </defs>
      <path :d="dimPath" fill-rule="evenodd" class="collection-scrim" />
      <rect
        :x="rect.left - 6"
        :y="rect.top - 6"
        :width="rect.width + 12"
        :height="rect.height + 12"
        rx="8"
        fill="none"
        stroke="var(--collection-paper)"
        stroke-width="7"
      />
      <rect
        :x="rect.left - 6"
        :y="rect.top - 6"
        :width="rect.width + 12"
        :height="rect.height + 12"
        rx="8"
        fill="none"
        stroke="var(--collection-marker)"
        stroke-width="3"
      />
      <polyline
        v-if="placement"
        :points="arrow"
        fill="none"
        stroke="var(--collection-marker)"
        stroke-width="2"
        marker-end="url(#collection-arrow)"
      />
    </svg>
  </Teleport>
  <Teleport :to="portal">
    <Panel
      v-if="request && rect && placement"
      ref="coach"
      class="collection-coach collection-theme"
      data-collection-coach
      role="note"
      aria-label="Document collection hint"
      :style="{
        left: `${placement.x - origin.left}px`,
        top: `${placement.y - origin.top}px`,
        width: `${placement.width}px`,
      }"
    >
      <template #header
        ><p class="collection-coach-title">{{ request.title }}</p></template
      >
      <template #icons
        ><Button
          icon="oi oi-times"
          text
          rounded
          severity="secondary"
          aria-label="Close hint"
          title="Close hint (Esc)"
          @click="close()"
      /></template>
      <p id="collection-coach-description">{{ request.body }}</p>
      <span class="collection-coach-escape">Esc to close</span>
    </Panel>
    <span v-else-if="request && rect" id="collection-coach-description" class="sr-only"
      >{{ request.body }} Press Escape to close this hint.</span
    >
  </Teleport>
</template>

<style>
.collection-spotlight {
  position: fixed;
  inset: 0;
  width: 100%;
  height: 100%;
  z-index: 1200;
  pointer-events: none;
}
.collection-scrim {
  fill: var(--collection-dim);
}
.collection-coach {
  position: fixed;
  z-index: 1201;
  border-top: 3px solid var(--collection-marker);
  box-shadow: var(--collection-lift);
  border-radius: 12px;
}
.collection-coach .p-panel-header {
  padding: 8px 8px 0 16px;
}
.collection-coach .p-panel-content {
  padding: 0 16px 16px;
}
.collection-coach .p-button {
  width: 40px;
  height: 40px;
}
.collection-coach-title {
  font-weight: 650;
  font-size: 15px;
  color: var(--collection-ink);
  margin: 0;
}
.collection-coach #collection-coach-description {
  font-size: 13px;
  line-height: 1.5;
  color: var(--collection-secondary);
  margin: 0 0 12px;
  text-wrap: pretty;
}
.collection-coach-escape {
  font-size: 11px;
  color: var(--collection-muted);
}
@media (max-width: 600px) {
  .collection-coach .p-panel-header {
    padding: 4px 4px 0 12px;
  }
  .collection-coach .p-panel-content {
    padding: 0 12px 12px;
  }
  .collection-coach .p-button {
    width: 44px;
    height: 44px;
  }
  .collection-coach-title {
    font-size: 14px;
  }
  .collection-coach #collection-coach-description {
    font-size: 12px;
    margin-bottom: 0;
  }
  .collection-coach-escape {
    display: none;
  }
}
</style>
