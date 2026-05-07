import axios from 'axios';
import { defineStore } from 'pinia';

import type { SecureLink } from '@/types/models';

export const useSecureLinksStore = defineStore('secureLinks', () => {
  const links = ref<SecureLink[]>([]);
  const loading = ref(false);
  const error = ref<string | null>(null);

  async function fetchLinks(clientId: string) {
    loading.value = true;
    error.value = null;
    try {
      const res = await axios.get(`/api/clients/${clientId}/links`);
      links.value = res.data;
    } catch (e: any) {
      error.value = e.response?.data?.error || 'Failed to fetch links';
    } finally {
      loading.value = false;
    }
  }

  async function createLink(clientId: string, accessType: string, expiryDays: number, email = '') {
    loading.value = true;
    error.value = null;
    try {
      const res = await axios.post(`/api/clients/${clientId}/links`, {
        access_type: accessType,
        expiry_days: expiryDays,
        email,
      });
      links.value.unshift(res.data);
      return res.data as SecureLink;
    } catch (e: any) {
      error.value = e.response?.data?.error || 'Failed to create link';
      throw e;
    } finally {
      loading.value = false;
    }
  }

  async function revokeLink(linkId: string) {
    loading.value = true;
    error.value = null;
    try {
      await axios.delete(`/api/links/${linkId}`);
      links.value = links.value.filter((link) => link.id !== linkId);
    } catch (e: any) {
      error.value = e.response?.data?.error || 'Failed to revoke link';
      throw e;
    } finally {
      loading.value = false;
    }
  }

  return {
    links,
    loading,
    error,
    fetchLinks,
    createLink,
    revokeLink,
  };
});
