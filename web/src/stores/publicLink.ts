import axios from 'axios';
import { defineStore } from 'pinia';

import type { Client, FileItem, SecureLink } from '@/types/models';

export const usePublicLinkStore = defineStore('publicLink', () => {
  const client = ref<Client | null>(null);
  const clientId = computed(() => client.value?.id ?? null);

  const link = ref<SecureLink | null>(null);
  const linkToken = ref('');
  const target = ref<FileItem | null>(null);
  let validationVersion = 0;
  const validating = ref(false);
  const error = ref<string | null>(null);
  const registerError = ref<string | null>(null);
  const registerSuccess = ref(false);

  const accessType = computed(() => link.value?.access_type ?? 'read');
  const available = computed(
    () =>
      !!link.value &&
      !validating.value &&
      !error.value &&
      Date.parse(link.value.expires_at) > Date.now(),
  );
  const canRead = computed(
    () => available.value && ['read', 'view', 'readwrite'].includes(accessType.value),
  );
  const canWrite = computed(
    () => available.value && ['write', 'readwrite'].includes(accessType.value),
  );
  const canRegister = computed(
    () => available.value && !link.value?.file_id && accessType.value === 'readwrite',
  );

  function getErrorMessage(err: unknown, fallback: string) {
    if (axios.isAxiosError(err)) {
      return err.response?.data?.error || fallback;
    }
    return fallback;
  }

  async function validateLink(token: string) {
    validationVersion += 1;
    const version = validationVersion;
    client.value = null;
    link.value = null;
    target.value = null;
    validating.value = true;
    error.value = null;
    registerError.value = null;
    registerSuccess.value = false;
    linkToken.value = token;
    try {
      const res = await axios.get(`/api/link/${token}`);
      if (version !== validationVersion) {
        return;
      }
      client.value = res.data.client;
      link.value = res.data.link;
      target.value = res.data.target ?? null;
    } catch (e: unknown) {
      if (version !== validationVersion) {
        return;
      }
      error.value = getErrorMessage(e, 'Failed to validate link');
      client.value = null;
      link.value = null;
    } finally {
      if (version === validationVersion) {
        validating.value = false;
      }
    }
  }

  async function registerUser(name: string, email: string, password: string) {
    if (!linkToken.value || !canRegister.value) {
      registerError.value =
        'This link does not allow account registration. Ask for a named invitation.';
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
    target,
    validating,
    error,
    registerError,
    registerSuccess,
    accessType,
    canRead,
    canWrite,
    canRegister,

    validateLink,
    registerUser,
  };
});
