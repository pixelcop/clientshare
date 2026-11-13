import * as Sentry from '@sentry/vue';
import axios from 'axios';

export interface ReplayPolicy {
  enabled: boolean;
  start_on_failure: boolean;
  expires_at?: string;
}

interface Replay {
  start: () => void;
  stop: (options: { flush: boolean }) => Promise<unknown>;
  active?: () => boolean;
}

// Kept independent of Vue/auth state so anonymous startup and policy races are
// testable. The server is the sole authority for eligibility and expiry.
export class RuntimeReplay {
  private policy: ReplayPolicy = { enabled: false, start_on_failure: false };

  private recording = false;

  private generation = 0;

  private awaitingPolicy = true;

  private pendingFailure = false;

  private expiryTimer?: ReturnType<typeof setTimeout>;

  private operations = Promise.resolve();

  constructor(
    private replay: Replay,
    private load: () => Promise<ReplayPolicy>,
    private safeToRecord: () => boolean = () => true,
  ) {}

  private eligible() {
    const expiry = Date.parse(this.policy.expires_at || '');
    return (
      this.policy.enabled && Number.isFinite(expiry) && expiry > Date.now() && this.safeToRecord()
    );
  }

  private reconcile() {
    this.operations = this.operations
      .then(async () => {
        const shouldRecord =
          this.eligible() &&
          (!this.policy.start_on_failure || this.recording || this.pendingFailure);
        if (!shouldRecord && (this.recording || this.replay.active?.() === true)) {
          // Never upload a pending segment after eligibility was withdrawn.
          await this.replay.stop({ flush: false });
          this.recording = false;
        } else if (shouldRecord && (!this.recording || this.replay.active?.() === false)) {
          this.replay.start();
          this.recording = true;
        }
        this.pendingFailure = false;
      })
      .catch(() => {
        /* Diagnostics must never break the application. */
      });
    return this.operations;
  }

  async refresh() {
    this.generation += 1;
    const { generation } = this;
    this.awaitingPolicy = true;
    let policy: ReplayPolicy;
    try {
      policy = await this.load();
    } catch {
      policy = { enabled: false, start_on_failure: false };
    }
    if (generation !== this.generation) {
      return;
    }
    this.policy = policy;
    this.awaitingPolicy = false;
    clearTimeout(this.expiryTimer);
    if (this.eligible()) {
      // Clamp untrusted/malformed responses and recheck the policy at expiry.
      const delay = Math.min(Date.parse(policy.expires_at!) - Date.now(), 24 * 60 * 60 * 1000);
      this.expiryTimer = setTimeout(() => {
        void this.reconcile().then(() => this.refresh());
      }, delay);
    }
    await this.reconcile();
  }

  failure(status: number) {
    if (status < 400 || status >= 600) {
      return;
    }
    if (this.awaitingPolicy) {
      this.pendingFailure = true;
    }
    if (this.eligible()) {
      this.pendingFailure = true;
      void this.reconcile();
    }
  }

  async suspend() {
    this.generation += 1;
    this.awaitingPolicy = false;
    this.policy = { enabled: false, start_on_failure: false };
    this.pendingFailure = false;
    clearTimeout(this.expiryTimer);
    await this.reconcile();
  }
}

let controller: RuntimeReplay | undefined;
let refreshing: Promise<void> | undefined;

// DOM snapshots include the current URL. Do not start on credential-bearing
// bootstrap pages; recording can resume after the application consumes the URL.
export function isReplayURLSafe(value: string) {
  try {
    const url = new URL(value, window.location.origin);
    const sensitive =
      /token|handoff|password|secret|signature|authorization|session|email|^code$|^key$|^sig$/i;
    const parameters = [...url.searchParams, ...new URLSearchParams(url.hash.slice(1))];
    return (
      !url.pathname.startsWith('/links/') &&
      !parameters.some(
        ([key, entry]) =>
          sensitive.test(key) ||
          /(?:token|handoff|password|secret|signature|authorization)=/i.test(entry),
      )
    );
  } catch {
    return false;
  }
}

export function identifyReplayUser(user: { id: string; tenant_id?: string } | null) {
  Sentry.setUser(user ? { id: user.id } : null);
  Sentry.setTag('tenant_id', user?.tenant_id);
}

export function refreshRuntimeReplay() {
  if (!refreshing && controller) {
    const request = controller.refresh();
    refreshing = request;
    void request.finally(() => {
      if (refreshing === request) {refreshing = undefined;}
    });
  }
  return refreshing;
}

export function suspendRuntimeReplay() {
  refreshing = undefined;
  return controller?.suspend();
}

function isApplicationAPI(value: string) {
  try {
    const url = new URL(value, window.location.origin);
    return (
      url.origin === window.location.origin &&
      url.pathname.startsWith('/api/') &&
      url.pathname !== '/api/config'
    );
  } catch {
    return false;
  }
}

export function startRuntimeReplay() {
  if (controller) {
    return;
  }
  const replay = Sentry.getReplay();
  const originalFetch = window.fetch.bind(window);
  controller = new RuntimeReplay(
    {
      start: () => {
        replay?.start();
      },
      active: () => !!replay?.getReplayId(),
      stop: async (options) => {
        await replay?.stop(options);
      },
    },
    async () => {
      const response = await originalFetch('/api/config', {
        credentials: 'same-origin',
        cache: 'no-store',
        signal: AbortSignal.timeout(5000),
      });
      if (!response.ok) {
        throw new Error('Configuration unavailable');
      }
      const config = await response.json();
      return config.debug?.replay ?? { enabled: false, start_on_failure: false };
    },
    () => isReplayURLSafe(window.location.href),
  );

  // HTTP failures are often handled by the UI, not captured exceptions. Observe
  // both API transports without uploading request bodies or credentials.
  axios.interceptors.response.use(
    (response) => {
      if (isApplicationAPI(response.config.url || '')) {
        controller?.failure(response.status);
      }
      return response;
    },
    (error) => {
      if (isApplicationAPI(error.config?.url || '')) {
        controller?.failure(error.response?.status || 0);
      }
      return Promise.reject(error);
    },
  );
  window.fetch = async (...args: Parameters<typeof fetch>) => {
    const response = await originalFetch(...args);
    const input = args[0];
    let url: string;
    if (typeof input === 'string') {
      url = input;
    } else if (input instanceof URL) {
      url = input.href;
    } else {
      url = input.url;
    }
    if (isApplicationAPI(url)) {
      controller?.failure(response.status);
    }
    return response;
  };
  void refreshRuntimeReplay();
  setInterval(() => {
    void refreshRuntimeReplay();
  }, 30_000);
  window.addEventListener('pageshow', (event) => {
    if (event.persisted) {void refreshRuntimeReplay();}
  });
}
