import axios from 'axios';
import { defineStore } from 'pinia';
import { computed, ref, watch } from 'vue';

import { useAuthStore } from '@/stores/auth';
import type { Client } from '@/types/models';

export interface CollectionLink {
  id: string;
  access_type: string;
  created_at: string;
  expires_at: string;
  received: boolean;
}
export interface CollectionProgress {
  started: string;
  dismissed: boolean;
  collapsed: boolean;
  clientId?: string;
  linkId?: string;
  shared?: 'copied' | 'email_queued';
}
export type CollectionCue = 'start' | 'client-name' | 'link-access' | 'copy';
export function usableUploadLink(link: CollectionLink, now = Date.now()) {
  return ['write', 'readwrite'].includes(link.access_type) && Date.parse(link.expires_at) > now;
}
export function collectionStep(
  client: Client | null,
  link: CollectionLink | undefined,
  shared?: string,
) {
  if (!client) {
    return 0;
  }
  if (!link) {
    return 1;
  }
  if (link.received) {
    return 4;
  }
  if (!usableUploadLink(link)) {
    return 1;
  }
  return shared ? 3 : 2;
}
export function guideTopic(base: string, topic: string) {
  const url = new URL(base || '/docs/', window.location.origin);
  url.search = '';
  url.hash = '';
  url.pathname = `${url.pathname.replace(/index\.html$/, '').replace(/\/$/, '')}/${topic}.html`;
  return url.href;
}

export const useCollectionStore = defineStore('collection', () => {
  const auth = useAuthStore();
  const allowed = computed(() => auth.user?.role === 'admin');
  const key = computed(() =>
    auth.user
      ? `clientshare:collect:v1:${auth.user.tenant_id || window.location.host}:${auth.user.id}`
      : '',
  );
  const progress = ref<CollectionProgress | null>(null);
  const client = ref<Client | null>(null);
  const links = ref<CollectionLink[]>([]);
  const error = ref('');
  const loading = ref(false);
  const guideBase = ref('');
  // Guidance is an explicit, current-session interaction; never replay it on restore.
  const guidance = ref(false);
  const cue = ref<{ kind: CollectionCue } | null>(null);
  function guide(kind: CollectionCue) {
    guidance.value = true;
    cue.value = { kind };
  }
  function stopGuidance() {
    guidance.value = false;
    cue.value = null;
  }
  const link = computed(() => links.value.find((item) => item.id === progress.value?.linkId));
  const step = computed(() => collectionStep(client.value, link.value, progress.value?.shared));
  let generation = 0;
  function restore(preserveMissing = false) {
    try {
      const raw = localStorage.getItem(key.value);
      if (!raw && preserveMissing) {
        return;
      }
      const value = JSON.parse(raw || 'null');
      progress.value =
        value && typeof value.started === 'string' && Number.isFinite(Date.parse(value.started))
          ? {
              started: value.started,
              dismissed: !!value.dismissed,
              collapsed: !!value.collapsed,
              clientId: typeof value.clientId === 'string' ? value.clientId : undefined,
              linkId: typeof value.linkId === 'string' ? value.linkId : undefined,
              shared: ['copied', 'email_queued'].includes(value.shared) ? value.shared : undefined,
            }
          : null;
    } catch {
      if (!preserveMissing) {
        progress.value = null;
      }
    }
  }
  function save(change: Partial<CollectionProgress>) {
    if (!allowed.value) {
      return;
    }
    // Read the latest tab's context before applying this action.
    restore(true);
    progress.value = {
      started: new Date().toISOString(),
      dismissed: false,
      collapsed: false,
      ...progress.value,
      ...change,
    };
    try {
      localStorage.setItem(key.value, JSON.stringify(progress.value));
    } catch {
      /* session-only when storage is unavailable */
    }
  }
  async function reconcile() {
    generation += 1;
    const run = generation;
    error.value = '';
    loading.value = false;
    const context = progress.value;
    if (!allowed.value || !context?.clientId) {
      client.value = null;
      links.value = [];
      return;
    }
    loading.value = true;
    try {
      const [c, l] = await Promise.all([
        axios.get<Client>(`/api/clients/${context.clientId}`),
        axios.get<CollectionLink[]>(`/api/clients/${context.clientId}/collection`, {
          params: { since: context.started },
        }),
      ]);
      if (run !== generation) {
        return;
      }
      client.value = c.data;
      links.value = l.data;
      if (!context.linkId) {
        const recent = l.data.find(
          (item) =>
            usableUploadLink(item) && Date.parse(item.created_at) >= Date.parse(context.started),
        );
        if (recent) {
          save({ linkId: recent.id });
        }
      }
    } catch (e) {
      if (run !== generation) {
        return;
      }
      client.value = null;
      links.value = [];
      if (axios.isAxiosError(e) && e.response?.status === 404) {
        save({ clientId: undefined, linkId: undefined, shared: undefined });
        error.value = 'This client is no longer available. Choose another client.';
      } else {
        error.value = 'Could not refresh progress. Check your access and try again.';
      }
    } finally {
      if (run === generation) {
        loading.value = false;
      }
    }
  }
  function selectClient(clientId: string) {
    save({ clientId, linkId: undefined, shared: undefined, started: new Date().toISOString() });
    void reconcile();
  }
  function start(clientId?: string) {
    save({ dismissed: false, collapsed: false });
    if (clientId && clientId !== progress.value?.clientId) {
      selectClient(clientId);
    } else {
      void reconcile();
    }
    guide('start');
  }
  function selectLink(id: string) {
    save({ linkId: id, shared: undefined });
  }
  function shared(clientId: string, linkId: string, method: 'copied' | 'email_queued') {
    if (progress.value?.clientId !== clientId) {
      return;
    }
    save({ linkId, shared: method });
    void reconcile();
  }
  function restart() {
    save({
      started: new Date().toISOString(),
      clientId: undefined,
      linkId: undefined,
      shared: undefined,
      dismissed: false,
      collapsed: false,
    });
    void reconcile();
    guide('start');
  }
  watch(
    key,
    () => {
      restore();
      void reconcile();
    },
    { immediate: true },
  );
  watch(allowed, () => {
    void reconcile();
  });
  window.addEventListener('storage', (event) => {
    if (event.key === key.value || event.key === null) {
      restore();
      void reconcile();
    }
  });
  return {
    allowed,
    progress,
    client,
    links,
    link,
    step,
    loading,
    error,
    guideBase,
    guidance,
    cue,
    guide,
    stopGuidance,
    start,
    save,
    selectClient,
    selectLink,
    shared,
    restart,
    reconcile,
  };
});
