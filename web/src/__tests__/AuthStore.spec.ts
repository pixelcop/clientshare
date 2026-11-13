import axios from 'axios';
import { createPinia, disposePinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import * as help from '../services/helpCapabilities';
import * as replay from '../services/runtimeReplay';
import { useAuthStore } from '../stores/auth';

describe('auth store', () => {
  let pinia: ReturnType<typeof createPinia>;
  beforeEach(() => {
    localStorage.removeItem('jwt');
    pinia = createPinia();
    setActivePinia(pinia);
    vi.restoreAllMocks();
  });
  afterEach(() => {
    disposePinia(pinia);
    vi.useRealTimers();
  });

  const identity = { id: 'user-1', tenant_id: 'tenant', role: 'admin', session_id: 'session' };

  async function signedIn() {
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
    const get = vi.spyOn(axios, 'get').mockResolvedValue({ data: { ...identity } });
    const auth = useAuthStore();
    await auth.fetchMe(true);
    get.mockClear();
    return { auth, get };
  }

  it('does not recheck auth on focus, visibility changes or page restores', async () => {
    vi.useFakeTimers();
    const { auth, get } = await signedIn();
    const { user } = auth;
    document.dispatchEvent(new Event('visibilitychange'));
    window.dispatchEvent(new Event('focus'));
    window.dispatchEvent(new PageTransitionEvent('pageshow', { persisted: true }));
    await vi.advanceTimersByTimeAsync(1000);
    expect(get).not.toHaveBeenCalled();
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');
    document.dispatchEvent(new Event('visibilitychange'));
    window.dispatchEvent(new Event('focus'));
    await vi.advanceTimersByTimeAsync(1000);
    expect(get).not.toHaveBeenCalled();
    expect(auth.user).toBe(user);
  });

  it('loads auth at startup, reuses it for navigation and allows explicit refreshes', async () => {
    const get = vi.spyOn(axios, 'get').mockResolvedValue({ data: { ...identity } });
    const auth = useAuthStore();
    await auth.fetchMe();
    expect(get).toHaveBeenCalledExactlyOnceWith('/api/auth/me');
    get.mockClear();
    await auth.fetchMe();
    expect(get).not.toHaveBeenCalled();
    await auth.fetchMe(true);
    expect(get).toHaveBeenCalledExactlyOnceWith('/api/auth/me');
  });

  it('refreshes diagnostics only after actual session changes, not startup or repeated 401s', async () => {
    const refresh = vi.spyOn(replay, 'refreshRuntimeReplay');
    const invalidate = vi.spyOn(help, 'invalidateHelpCapabilities');
    const { auth } = await signedIn();
    await auth.fetchMe(true);
    expect(refresh).not.toHaveBeenCalled();
    expect(invalidate).not.toHaveBeenCalled();
    vi.mocked(axios.get).mockRejectedValue(new Error('Unauthorized'));
    await auth.fetchMe(true);
    await auth.fetchMe(true);
    expect(refresh).toHaveBeenCalledTimes(1);
    expect(invalidate).toHaveBeenCalledTimes(1);
    vi.mocked(axios.get).mockResolvedValue({ data: identity });
    await auth.fetchMe(true);
    expect(refresh).toHaveBeenCalledTimes(2);
  });

  it('clears auth on an API 401 without sending a logout request', async () => {
    const { auth, get } = await signedIn();
    get.mockRestore();
    const post = vi.spyOn(axios, 'post');
    await expect(
      axios.get('/api/protected', {
        adapter: async () => {
          throw Object.assign(new Error('Unauthorized'), { response: { status: 401 } });
        },
      }),
    ).rejects.toThrow('Unauthorized');
    expect(auth.user).toBeNull();
    expect(post).not.toHaveBeenCalled();
  });

  it('retains cross-tab session notifications and removes the listener on disposal', async () => {
    const add = vi.spyOn(window, 'addEventListener');
    const remove = vi.spyOn(window, 'removeEventListener');
    const { auth } = await signedIn();
    const storage = add.mock.calls.find(([event]) => event === 'storage');
    expect(storage).toBeDefined();
    expect(add.mock.calls.map(([event]) => event)).not.toContain('focus');
    auth.$dispose();
    expect(remove).toHaveBeenCalledWith('storage', storage![1]);
  });

  it('offers passkey enrollment after a hosted login handoff', async () => {
    vi.spyOn(axios, 'post').mockResolvedValue({ data: { passkey_enrollment: true } });
    vi.spyOn(axios, 'get').mockResolvedValue({
      data: { id: 'user-1', role: 'admin' },
    });
    const auth = useAuthStore();

    expect(await auth.acceptHostedLoginHandoff('handoff-code')).toBe(true);
    expect(auth.passkeyEnrollmentPrompt).toBe(true);
  });
});
