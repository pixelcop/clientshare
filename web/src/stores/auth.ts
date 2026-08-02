import axios from 'axios';
import { defineStore } from 'pinia';

import type { User } from '@/types/models';

// Global session expiry handler
let storeRef: { logout: () => Promise<void> | void } | null = null;
axios.defaults.withCredentials = true;

axios.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response && error.response.status === 401) {
      // If storeRef is set, log out
      if (storeRef && typeof storeRef.logout === 'function') {
        storeRef.logout();
      }
    }
    return Promise.reject(error);
  },
);

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null);
  const token = ref<string | null>(localStorage.getItem('jwt') || null);
  const loading = ref(false);
  const error = ref<string | null>(null);
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

  setAuthHeader(token.value);

  async function fetchMe(force = false): Promise<User | null> {
    if (user.value && !force) {
      return user.value; // already loaded
    }

    try {
      const res = await axios.get('/api/auth/me');
      user.value = res.data;
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

  async function acceptRedirectToken(jwt: string) {
    const trimmed = jwt.trim();
    if (!trimmed) {
      return false;
    }

    token.value = trimmed;
    setAuthHeader(trimmed);
    user.value = null;

    const authenticatedUser = await fetchMe();
    if (!authenticatedUser) {
      clearLegacyToken();
      user.value = null;
      return false;
    }

    return true;
  }

  async function logout() {
    try {
      await axios.post('/api/auth/logout');
    } catch {
      // ignore
    }
    clearLegacyToken();
    user.value = null;
    clearPasskeyEnrollmentPrompt();
  }

  // Set storeRef for global 401 handler
  storeRef = { logout };

  return {
    user,
    token,
    loading,
    error,
    passkeyEnrollmentPrompt,
    login,
    loginWithPasskey,
    acceptRedirectToken,
    logout,
    fetchMe,
    offerPasskeyEnrollment,
    clearPasskeyEnrollmentPrompt,
  };
});
