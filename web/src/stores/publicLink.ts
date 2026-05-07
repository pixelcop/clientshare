import axios from 'axios';
import { defineStore } from 'pinia';

import type { Client, SecureLink } from '@/types/models';

export const usePublicLinkStore = defineStore('publicLink', () => {
  const client = ref<Client | null>(null);
  const clientId = computed(() => client.value?.id ?? null);

  const link = ref<SecureLink | null>(null);
  const linkToken = ref('');
  const validating = ref(false);
  const error = ref<string | null>(null);
  const registerError = ref<string | null>(null);
  const registerSuccess = ref(false);

  const accessType = computed(() => link.value?.access_type ?? 'read');
  const canRead = computed(() => accessType.value !== 'write'); // TODO: is this right
  const canWrite = computed(() => accessType.value !== 'read');

  function getErrorMessage(err: unknown, fallback: string) {
    if (axios.isAxiosError(err)) {
      return err.response?.data?.error || fallback;
    }
    return fallback;
  }

  async function validateLink(token: string) {
    validating.value = true;
    error.value = null;
    registerError.value = null;
    registerSuccess.value = false;
    linkToken.value = token;
    try {
      const res = await axios.get(`/api/link/${token}`);
      client.value = res.data.client;
      link.value = res.data.link;
    } catch (e: unknown) {
      error.value = getErrorMessage(e, 'Failed to validate link');
      client.value = null;
      link.value = null;
    } finally {
      validating.value = false;
    }
  }

  async function registerUser(name: string, email: string, password: string) {
    if (!linkToken.value) {
      return false;
    }
    registerError.value = null;
    registerSuccess.value = false;
    try {
      await axios.post('/api/auth/register', {
        token: linkToken.value,
        name,
        email,
        password,
      });
      registerSuccess.value = true;
      return true;
    } catch (e: unknown) {
      registerError.value = getErrorMessage(e, 'Failed to create account');
      return false;
    }
  }

  return {
    client,
    clientId,
    link,
    linkToken,
    validating,
    error,
    registerError,
    registerSuccess,
    accessType,
    canRead,
    canWrite,

    validateLink,
    registerUser,
  };
});
