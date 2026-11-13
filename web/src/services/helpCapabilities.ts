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

let cached: Promise<HelpCapabilities> | undefined;

export function invalidateHelpCapabilities() {
  cached = undefined;
}

// Share both pending requests and successful results for the current session.
// Failed requests are retryable; an old failure cannot evict a new session.
export function fetchHelpCapabilities() {
  if (!cached) {
    const request = axios.get<HelpCapabilities>('/api/help').then(({ data }) => data);
    cached = request;
    void request.catch(() => {
      if (cached === request) {cached = undefined;}
    });
  }
  return cached;
}
