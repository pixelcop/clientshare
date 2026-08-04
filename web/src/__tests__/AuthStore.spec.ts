import axios from 'axios';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useAuthStore } from '../stores/auth';

describe('auth store', () => {
  beforeEach(() => {
    localStorage.removeItem('jwt');
    setActivePinia(createPinia());
    vi.restoreAllMocks();
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
