import axios from 'axios';
import { onBeforeUnmount, ref, watch } from 'vue';

import type { RecipientContact } from '@/types/recipients';

// No shared cache: changing actor, client, or scope must not reuse a directory
// response from a wider authorization context.
export function useRecipientSearch(
  context: () => { clientId: string; enabled: boolean; actorKey: string },
) {
  const contacts = ref<RecipientContact[]>([]);
  const across = ref(false);
  const loading = ref(false);
  const error = ref('');
  const available = ref(true);
  let controller: AbortController | undefined;
  let generation = 0;

  function reset() {
    generation += 1;
    controller?.abort();
    contacts.value = [];
    loading.value = false;
    error.value = '';
  }

  async function search(query: string) {
    reset();
    const current = context();
    if (!current.enabled || !available.value || (!current.clientId && !across.value)) {
      return;
    }
    const request = generation;
    controller = new AbortController();
    loading.value = true;
    try {
      const { data } = await axios.get<RecipientContact[]>('/api/recipients', {
        params: {
          client_id: current.clientId,
          scope: across.value ? 'all' : 'client',
          search: query.trim().slice(0, 200),
        },
        signal: controller.signal,
      });
      if (request === generation) {
        contacts.value = data;
      }
    } catch (cause) {
      if (request !== generation || axios.isCancel(cause)) {
        return;
      }
      if (axios.isAxiosError(cause) && [403, 404].includes(cause.response?.status || 0)) {
        available.value = false;
      } else {
        error.value = 'Contacts could not be loaded. You can still enter an email address.';
      }
    } finally {
      if (request === generation) {
        loading.value = false;
      }
    }
  }

  watch(
    () => [context().clientId, context().enabled, context().actorKey],
    () => {
      reset();
      across.value = false;
      available.value = true;
    },
    { flush: 'sync' },
  );
  watch(across, reset, { flush: 'sync' });
  onBeforeUnmount(reset);
  return { contacts, across, loading, error, available, search, reset };
}
