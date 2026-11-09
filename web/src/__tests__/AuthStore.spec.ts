import axios from 'axios';
import { createPinia, disposePinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

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

  it('coalesces visibility and focus into one auth check and updates account details', async () => {
    vi.useFakeTimers();
    const { auth, get } = await signedIn();
    get.mockResolvedValue({ data: { ...identity, name: 'Updated name' } });
    document.dispatchEvent(new Event('visibilitychange'));
    await vi.advanceTimersByTimeAsync(40);
    window.dispatchEvent(new Event('focus'));
    await vi.advanceTimersByTimeAsync(99);
    expect(get).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(get).toHaveBeenCalledExactlyOnceWith('/api/auth/me');
    expect(auth.user?.name).toBe('Updated name');
    expect(auth.sessionCheckVersion).toBe(1);
    window.dispatchEvent(new Event('focus'));
    await vi.advanceTimersByTimeAsync(100);
    expect(get).toHaveBeenCalledTimes(2);
  });

  it('shares an in-flight session check and ignores extra focus events during it', async () => {
    vi.useFakeTimers();
    const { auth, get } = await signedIn();
    let finish!: (value: { data: typeof identity }) => void;
    get.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const first = auth.revalidateSession();
    const second = auth.revalidateSession();
    window.dispatchEvent(new Event('focus'));
    document.dispatchEvent(new Event('visibilitychange'));
    await vi.advanceTimersByTimeAsync(100);
    expect(get).toHaveBeenCalledTimes(1);
    finish({ data: { ...identity } });
    expect(await first).toBe(true);
    expect(await second).toBe(true);
    expect(auth.sessionCheckVersion).toBe(1);
  });

  it('keeps the current identity on a transient failure and permits a retry', async () => {
    const { auth, get } = await signedIn();
    get.mockRejectedValueOnce(new Error('Temporarily unavailable'));
    expect(await auth.revalidateSession()).toBe(false);
    expect(auth.user?.id).toBe(identity.id);
    expect(auth.sessionCheckVersion).toBe(0);
    expect(await auth.revalidateSession()).toBe(true);
    expect(get).toHaveBeenCalledTimes(2);
  });

  it('checks restored pages, but not initial pageshow or hidden documents', async () => {
    vi.useFakeTimers();
    const { get } = await signedIn();
    window.dispatchEvent(new PageTransitionEvent('pageshow', { persisted: false }));
    await vi.advanceTimersByTimeAsync(100);
    expect(get).not.toHaveBeenCalled();
    window.dispatchEvent(new PageTransitionEvent('pageshow', { persisted: true }));
    await vi.advanceTimersByTimeAsync(100);
    expect(get).toHaveBeenCalledTimes(1);
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');
    document.dispatchEvent(new Event('visibilitychange'));
    window.dispatchEvent(new Event('focus'));
    await vi.advanceTimersByTimeAsync(100);
    expect(get).toHaveBeenCalledTimes(1);
  });

  it('removes focus listeners and scheduled checks when the store is disposed', async () => {
    vi.useFakeTimers();
    const { auth, get } = await signedIn();
    window.dispatchEvent(new Event('focus'));
    auth.$dispose();
    window.dispatchEvent(new Event('focus'));
    document.dispatchEvent(new Event('visibilitychange'));
    await vi.advanceTimersByTimeAsync(100);
    expect(get).not.toHaveBeenCalled();
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
