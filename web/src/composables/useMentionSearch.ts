import axios from 'axios';
import { onScopeDispose, watch } from 'vue';

import type { CommentPerson } from '@/utils/commentEditor';
import { noPeopleMatch, normalizePeopleSearch, peopleMatch } from '@/utils/peopleSearch';

const cacheLifetime = 120_000;
const queryLimit = 24;

export function useMentionSearch(context: () => { fileId: string; sessionKey: string }) {
  interface Entry {
    people: CommentPerson[];
    expires: number;
  }
  interface Request {
    controller: AbortController;
    result: Promise<CommentPerson[]>;
  }
  const scopes = new Map<string, Map<string, Entry>>();
  const pending = new Map<string, Request>();
  const compare = (a: string, b: string) => Number(a > b) - Number(a < b);

  function reset() {
    scopes.clear();
    pending.forEach(({ controller }) => controller.abort());
    pending.clear();
  }
  watch(() => [context().fileId, context().sessionKey], reset, { flush: 'sync' });
  onScopeDispose(reset);

  function entries(visibility: string) {
    let cache = scopes.get(visibility);
    if (!cache) {
      cache = new Map();
      scopes.set(visibility, cache);
    }
    cache.forEach((entry, key) => {
      if (entry.expires <= Date.now()) {
        cache.delete(key);
      }
    });
    return cache;
  }

  function peek(visibility: string, query: string) {
    return entries(visibility).get(normalizePeopleSearch(query))?.people;
  }

  function local(visibility: string, query: string) {
    const key = normalizePeopleSearch(query);
    const cache = entries(visibility);
    const exact = cache.get(key);
    if (exact) {
      return exact.people;
    }
    const unique = new Map<string, CommentPerson>();
    cache.forEach(({ people }) => people.forEach((person) => unique.set(person.id, person)));
    return Array.from(unique.values(), (person) => ({
      person,
      rank: peopleMatch(key, person.name, person.email || ''),
      name: normalizePeopleSearch(person.name),
    }))
      .filter(({ rank }) => rank < noPeopleMatch)
      .sort(
        (a, b) =>
          a.rank - b.rank ||
          Number(!!b.person.previously_mentioned) - Number(!!a.person.previously_mentioned) ||
          compare(a.name, b.name) ||
          compare(a.person.id, b.person.id),
      )
      .slice(0, 8)
      .map(({ person }) => person);
  }

  function search(visibility: string, query: string) {
    const key = normalizePeopleSearch(query);
    const cached = peek(visibility, query);
    if (cached) {
      return Promise.resolve(cached);
    }
    const requestKey = `${visibility}:${key}`;
    const existing = pending.get(requestKey);
    if (existing) {
      return existing.result;
    }

    const controller = new AbortController();
    const result = axios
      .get<CommentPerson[]>(`/api/files/${context().fileId}/comments/mentions`, {
        params: { search: query.trim(), visibility },
        signal: controller.signal,
      })
      .then(({ data }) => {
        if (controller.signal.aborted) {
          return [];
        }
        const cache = entries(visibility);
        cache.set(key, { people: data, expires: Date.now() + cacheLifetime });
        if (cache.size > queryLimit) {
          const oldest = Array.from(cache.keys()).find(
            (candidate) => candidate && candidate !== key,
          );
          if (oldest) {
            cache.delete(oldest);
          }
        }
        return data;
      })
      .catch((cause: unknown) => {
        if (controller.signal.aborted) {
          return [];
        }
        if (axios.isAxiosError(cause) && [401, 403, 404].includes(cause.response?.status || 0)) {
          reset();
          return [];
        }
        throw cause;
      })
      .finally(() => {
        if (pending.get(requestKey)?.controller === controller) {
          pending.delete(requestKey);
        }
      });
    pending.set(requestKey, { controller, result });
    return result;
  }

  function warm(visibility: string) {
    void search(visibility, '').catch(() => {
      // Background failures are retried when the editor opens the suggestions.
    });
  }

  return { peek, local, search, warm, reset };
}

export type MentionSearch = ReturnType<typeof useMentionSearch>;
