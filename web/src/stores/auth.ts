import axios from 'axios';
import { defineStore } from 'pinia';

import type { User } from '@/types/models';

// Global session expiry handler
let storeRef: { clear: () => void } | null = null;
axios.defaults.withCredentials = true;
const sessionChangeKey = 'clientshare-session-change';
function announceSessionChange() {
  try {
    localStorage.setItem(sessionChangeKey, crypto.randomUUID());
  } catch {
    /* Storage can be disabled. */
  }
}

axios.interceptors.response.use(
  (response) => {
    if (
      response.config.method === 'post' &&
      /\/auth\/(login|logout|register|accept-invite|reset-password|hosted-login\/exchange|passkeys\/login\/verify)$/.test(
        (response.config.url || '').split('?')[0],
      )
    ) {
      announceSessionChange();
    }
    return response;
  },
  (error) => {
    if (error.response && error.response.status === 401) {
      // Never send logout in response to an old tab/request's 401: it could
      // revoke the new session now held in the shared cookie.
      storeRef?.clear();
    }
    return Promise.reject(error);
  },
);

export const useAuthStore = defineStore('auth', () => {
  const user = ref<
    | (User & {
        is_initial_admin?: boolean;
        session_id?: string;
        link_id?: string;
        file_id?: string;
        client_id?: string;
        access_type?: string;
      })
    | null
  >(null);
  const token = ref<string | null>(null);
  let sessionID: string | null = null;
  const loading = ref(false);
  const error = ref<string | null>(null);
  const passkeyEnrollmentPrompt = ref(false);
  const sessionCheckVersion = ref(0);

  // Set token in axios headers
  function setAuthHeader(jwt: string | null) {
    if (jwt) {
      axios.defaults.headers.common.Authorization = `Bearer ${jwt}`;
    } else {
      delete axios.defaults.headers.common.Authorization;
    }
  }

  function clearLegacyToken() {
    token.value = null;
    localStorage.removeItem('jwt');
    setAuthHeader(null);
  }

  clearLegacyToken();

  function clear() {
    user.value = null;
    sessionID = null;
    clearLegacyToken();
    clearPasskeyEnrollmentPrompt();
  }

  function invalidatePage() {
    clear();
    window.location.replace('/');
  }

  function onStorage(event: StorageEvent) {
    if (event.key === sessionChangeKey) {
      invalidatePage();
    }
  }
  let revalidation: Promise<boolean> | null = null;
  let focusTimer: ReturnType<typeof setTimeout> | undefined;

  async function checkSession(): Promise<boolean> {
    if (document.visibilityState !== 'visible' || !sessionID) {
      return false;
    }
    const previous = sessionID;
    const previousUser = user.value;
    try {
      const response = await axios.get('/api/auth/me');
      // A response from before a local login/logout must not replace its result.
      if (previous !== sessionID) {
        return false;
      }
      if (
        previous !== response.data.session_id ||
        response.data.id !== previousUser?.id ||
        response.data.tenant_id !== previousUser?.tenant_id ||
        response.data.role !== previousUser?.role ||
        response.data.link_id !== previousUser?.link_id
      ) {
        invalidatePage();
        return false;
      }
      user.value = response.data;
      sessionCheckVersion.value += 1;
      return true;
    } catch (err) {
      if (axios.isAxiosError(err) && err.response?.status === 401) {
        invalidatePage();
      }
      return false;
    }
  }

  async function revalidateSession(): Promise<boolean> {
    if (revalidation) {
      return revalidation;
    }
    revalidation = checkSession();
    try {
      return await revalidation;
    } finally {
      revalidation = null;
    }
  }

  function scheduleRevalidation() {
    clearTimeout(focusTimer);
    if (document.visibilityState !== 'visible' || !sessionID || revalidation) {
      return;
    }
    // Returning to a window can emit visibilitychange and focus separately.
    // Coalesce that burst rather than rechecking once for each event.
    focusTimer = setTimeout(() => void revalidateSession(), 100);
  }
  function onPageShow(event: PageTransitionEvent) {
    if (event.persisted) {
      scheduleRevalidation();
    }
  }
  window.addEventListener('storage', onStorage);
  window.addEventListener('focus', scheduleRevalidation);
  window.addEventListener('pageshow', onPageShow);
  document.addEventListener('visibilitychange', scheduleRevalidation);
  onScopeDispose(() => {
    clearTimeout(focusTimer);
    window.removeEventListener('storage', onStorage);
    window.removeEventListener('focus', scheduleRevalidation);
    window.removeEventListener('pageshow', onPageShow);
    document.removeEventListener('visibilitychange', scheduleRevalidation);
  });

  async function fetchMe(force = false): Promise<User | null> {
    if (user.value && !force) {
      return user.value; // already loaded
    }

    try {
      const res = await axios.get('/api/auth/me');
      user.value = res.data;
      sessionID = res.data.session_id;
      if (token.value) {
        clearLegacyToken();
      }
      return user.value;
    } catch (e) {
      if (axios.isAxiosError(e) && e.message === 'Request aborted') {
        return null;
      }
      user.value = null;
      clearLegacyToken();
      return null;
    }
  }

  async function login(email: string, password: string) {
    loading.value = true;
    error.value = null;
    try {
      const response = await axios.post<{ passkey_enrollment?: boolean }>('/api/auth/login', {
        email,
        password,
      });
      clearLegacyToken();
      const authenticatedUser = await fetchMe(true);
      if (authenticatedUser && response.data.passkey_enrollment) {
        offerPasskeyEnrollment();
      }
      return authenticatedUser !== null;
    } catch (e: unknown) {
      error.value =
        axios.isAxiosError(e) && typeof e.response?.data?.error === 'string'
          ? e.response.data.error
          : 'Login failed';
      clearLegacyToken();
      user.value = null;
      return false;
    } finally {
      loading.value = false;
    }
  }

  async function loginWithPasskey(email: string, challengeId: string, credential: unknown) {
    loading.value = true;
    error.value = null;
    try {
      await axios.post('/api/auth/passkeys/login/verify', {
        email,
        challenge_id: challengeId,
        credential,
      });
      clearLegacyToken();
      return (await fetchMe(true)) !== null;
    } catch (e: unknown) {
      error.value =
        axios.isAxiosError(e) && typeof e.response?.data?.error === 'string'
          ? e.response.data.error
          : 'Passkey sign in failed';
      clearLegacyToken();
      user.value = null;
      return false;
    } finally {
      loading.value = false;
    }
  }

  function offerPasskeyEnrollment() {
    if (user.value && user.value.role !== 'link') {
      passkeyEnrollmentPrompt.value = true;
    }
  }

  function clearPasskeyEnrollmentPrompt() {
    passkeyEnrollmentPrompt.value = false;
  }

  async function acceptHostedLoginHandoff(code: string) {
    if (!code.trim()) {
      return false;
    }

    try {
      const response = await axios.post<{ passkey_enrollment?: boolean }>(
        '/api/auth/hosted-login/exchange',
        { code: code.trim() },
      );
      clearLegacyToken();
      const authenticatedUser = await fetchMe(true);
      if (authenticatedUser && response.data.passkey_enrollment) {
        offerPasskeyEnrollment();
      }
      return authenticatedUser !== null;
    } catch {
      clearLegacyToken();
      user.value = null;
      return false;
    }
  }

  async function logout() {
    await axios.post('/api/auth/logout');
    clear();
  }

  // Set storeRef for global 401 handler
  storeRef = { clear };

  return {
    user,
    token,
    loading,
    error,
    passkeyEnrollmentPrompt,
    login,
    loginWithPasskey,
    acceptHostedLoginHandoff,
    logout,
    fetchMe,
    revalidateSession,
    sessionCheckVersion,
    offerPasskeyEnrollment,
    clearPasskeyEnrollmentPrompt,
  };
});
