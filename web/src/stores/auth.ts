import axios from 'axios';
import { defineStore } from 'pinia';

import { invalidateHelpCapabilities } from '@/services/helpCapabilities';
import {
  identifyReplayUser,
  refreshRuntimeReplay,
  suspendRuntimeReplay,
} from '@/services/runtimeReplay';
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
  const loading = ref(false);
  const error = ref<string | null>(null);
  const canManageUsers = computed(
    () =>
      user.value?.role === 'admin' ||
      (user.value?.role === 'manager' && user.value.access_profile?.can_manage_users !== false),
  );
  const passkeyEnrollmentPrompt = ref(false);

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

  let replayIdentity: string | undefined;
  function syncDiagnostics(identity: { id: string; tenant_id?: string; session_id?: string } | null) {
    const key = identity ? JSON.stringify([identity.tenant_id, identity.id, identity.session_id]) : '';
    identifyReplayUser(identity);
    const changed = replayIdentity !== undefined && replayIdentity !== key;
    replayIdentity = key;
    // Startup config already uses the browser's session cookie. Only a later
    // identity/session transition needs another config evaluation.
    if (changed) {
      invalidateHelpCapabilities();
      void suspendRuntimeReplay();
      void refreshRuntimeReplay();
    }
  }

  function clear() {
    syncDiagnostics(null);
    user.value = null;
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
  window.addEventListener('storage', onStorage);
  onScopeDispose(() => {
    window.removeEventListener('storage', onStorage);
  });

  async function fetchMe(force = false): Promise<User | null> {
    if (user.value && !force) {
      return user.value; // already loaded
    }

    try {
      const res = await axios.get('/api/auth/me');
      user.value = res.data;
      syncDiagnostics(res.data);
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
      syncDiagnostics(null);
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
    await suspendRuntimeReplay();
    clear();
  }

  // Set storeRef for global 401 handler
  storeRef = { clear };

  return {
    user,
    canManageUsers,
    token,
    loading,
    error,
    passkeyEnrollmentPrompt,
    login,
    loginWithPasskey,
    acceptHostedLoginHandoff,
    logout,
    fetchMe,
    offerPasskeyEnrollment,
    clearPasskeyEnrollmentPrompt,
  };
});
