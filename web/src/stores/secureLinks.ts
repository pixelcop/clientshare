import axios from 'axios';
import { defineStore } from 'pinia';

import type { LinkEmailResult, LinkEmailStatus } from '@/types/linkAccess';
import type { SecureLink } from '@/types/models';

import { useAuthStore } from './auth';

export const useSecureLinksStore = defineStore('secureLinks', () => {
  const links = ref<SecureLink[]>([]);
  const loading = ref(false);
  const error = ref<string | null>(null);
  let listingVersion = 0;
  let listedClient: string | null = null;
  let pendingCreation: { fingerprint: string; key: string } | null = null;
  interface EmailAttempt {
    recipient: string;
    key: string;
    status: LinkEmailStatus | null;
    error: string;
    sending: boolean;
  }
  const emailAttempts = reactive<Record<string, EmailAttempt>>({});
  function emailAttempt(linkId: string) {
    emailAttempts[linkId] ||= {
      recipient: '',
      key: crypto.randomUUID(),
      status: null,
      error: '',
      sending: false,
    };
    return emailAttempts[linkId];
  }
  const auth = useAuthStore();
  watch(
    () => `${auth.user?.tenant_id}:${auth.user?.id}:${auth.user?.role}`,
    () => {
      listingVersion += 1;
      listedClient = null;
      links.value = [];
      pendingCreation = null;
      Object.keys(emailAttempts).forEach((key) => {
        delete emailAttempts[key];
      });
    },
  );

  async function fetchLinks(clientId: string) {
    listingVersion += 1;
    const version = listingVersion;
    if (listedClient !== clientId) {
      links.value = [];
    }
    listedClient = clientId;
    loading.value = true;
    error.value = null;
    try {
      const res = await axios.get<SecureLink[]>(`/api/clients/${clientId}/links`);
      if (version === listingVersion) {
        links.value = res.data || [];
      }
    } catch (e: unknown) {
      if (version === listingVersion) {
        error.value = (axios.isAxiosError(e) && e.response?.data?.error) || 'Failed to fetch links';
      }
    } finally {
      if (version === listingVersion) {
        loading.value = false;
      }
    }
  }

  async function createLink(
    clientId: string,
    accessType: string,
    expiryDays: number,
    email = '',
    expiresAt?: string,
  ) {
    const body = {
      access_type: accessType,
      expiry_days: expiryDays,
      email,
      ...(expiresAt ? { expires_at: expiresAt } : {}),
    };
    const fingerprint = JSON.stringify([clientId, body]);
    if (pendingCreation?.fingerprint !== fingerprint) {
      pendingCreation = { fingerprint, key: crypto.randomUUID() };
    }
    const request = pendingCreation;
    loading.value = true;
    error.value = null;
    try {
      const res = await axios.post<SecureLink & { email_queued: boolean }>(
        `/api/clients/${clientId}/links`,
        body,
        { headers: { 'Idempotency-Key': request.key } },
      );
      if (pendingCreation === request) {
        pendingCreation = null;
      }
      if (listedClient === clientId) {
        links.value = [res.data, ...links.value.filter((link) => link.id !== res.data.id)];
      }
      return res.data;
    } catch (e: unknown) {
      error.value =
        (axios.isAxiosError(e) && e.response?.data?.error) ||
        'Failed to create link. Retry will reuse this creation request.';
      throw e;
    } finally {
      loading.value = false;
    }
  }

  async function emailLink(linkId: string, email: string, requestKey: string) {
    const res = await axios.post<LinkEmailResult>(
      `/api/links/${linkId}/email`,
      { email },
      { headers: { 'Idempotency-Key': requestKey } },
    );
    return res.data;
  }

  async function revokeLink(linkId: string) {
    error.value = null;
    try {
      await axios.delete(`/api/links/${linkId}`);
      links.value = links.value.map((link) =>
        link.id === linkId
          ? { ...link, revoked_at: new Date().toISOString(), token: '', link_url: '' }
          : link,
      );
    } catch (e: unknown) {
      error.value = (axios.isAxiosError(e) && e.response?.data?.error) || 'Failed to revoke link';
      throw e;
    }
  }

  return { links, loading, error, fetchLinks, createLink, emailLink, emailAttempt, revokeLink };
});
