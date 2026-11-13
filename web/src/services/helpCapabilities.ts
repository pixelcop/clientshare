import axios from 'axios';

export interface HelpCapabilities {
  provider: string;
  guide_url?: string;
  support_url?: string;
  support_label?: string;
  reply_email?: string;
  requester_name?: string;
  booking_url?: string;
}

const cache = new Map<string, Promise<HelpCapabilities>>();

export function invalidateHelpCapabilities() {
  cache.clear();
}

// Share both pending requests and successful results for the current session.
// Failed requests are retryable; an old failure cannot evict a new session.
export function fetchHelpCapabilities(authenticated = true) {
  const endpoint = authenticated ? '/api/help' : '/api/help/login';
  let cached = cache.get(endpoint);
  if (!cached) {
    const request = axios.get<HelpCapabilities>(endpoint).then(({ data }) => data);
    cached = request;
    cache.set(endpoint, request);
    void request.catch(() => {
      if (cache.get(endpoint) === request) {cache.delete(endpoint);}
    });
  }
  return cached;
}
