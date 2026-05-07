import axios from 'axios';
import { defineStore } from 'pinia';

import type { Client, PaginatedResult, PaginationResponse } from '@/types/models';
import { defaultPagination } from '@/types/util';

export const useClientsStore = defineStore('clients', () => {
  const clients = ref<Client[]>([]);
  const listedClients = ref<Client[]>([]);
  const listingPagination = ref<PaginationResponse>(defaultPagination());
  const listingSearch = ref('');
  const listingLoading = ref(false);
  const error = ref<string | null>(null);
  const clientId = ref<string | null>(null);
  let loadingPromise: Promise<void> | null = null;

  const currentClient = computed(() => clients.value.find((c) => c.id === clientId.value) || null);

  async function fetchClients() {
    if (loadingPromise) {
      return loadingPromise;
    }
    loadingPromise = new Promise(async (resolve) => {
      error.value = null;
      try {
        const res = await axios.get('/api/clients');
        clients.value = res.data;
      } catch (e: any) {
        error.value = e.response?.data?.error || 'Failed to fetch clients';
      } finally {
        loadingPromise = null;
        resolve();
      }
    });

    return loadingPromise;
  }

  async function fetchClientPage(
    page = 1,
    pageSize = listingPagination.value.page_size,
    search = '',
  ) {
    listingLoading.value = true;
    error.value = null;
    listingSearch.value = search;
    try {
      const res = await axios.get<PaginatedResult<Client>>('/api/clients', {
        params: {
          page,
          page_size: pageSize,
          search,
        },
      });
      listedClients.value = res.data.items || [];
      listingPagination.value = res.data.pagination || defaultPagination();
    } catch (e: any) {
      error.value = e.response?.data?.error || 'Failed to fetch clients';
    } finally {
      listingLoading.value = false;
    }
  }

  async function refreshClientPage() {
    await fetchClientPage(
      listingPagination.value.page,
      listingPagination.value.page_size,
      listingSearch.value,
    );
  }

  async function createClient(name: string) {
    error.value = null;
    try {
      const res = await axios.post('/api/clients', { name });
      clients.value.push(res.data);
      await refreshClientPage();
      return res.data;
    } catch (e: any) {
      error.value = e.response?.data?.error || 'Failed to create client';
      throw e;
    }
  }

  async function updateClient(id: string, name: string) {
    error.value = null;
    try {
      const res = await axios.put(`/api/clients/${id}`, { name });
      const idx = clients.value.findIndex((c) => c.id === id);
      if (idx !== -1) {
        clients.value[idx] = res.data;
      }
      await refreshClientPage();
      return res.data;
    } catch (e: any) {
      error.value = e.response?.data?.error || 'Failed to update client';
      throw e;
    }
  }

  async function deleteClient(id: string) {
    error.value = null;
    try {
      await axios.delete(`/api/clients/${id}?confirm=true`);
      clients.value = clients.value.filter((c) => c.id !== id);
      await refreshClientPage();
    } catch (e: any) {
      error.value = e.response?.data?.error || 'Failed to delete client';
      throw e;
    }
  }

  fetchClients();

  return {
    clients,
    listedClients,
    listingPagination,
    listingSearch,
    listingLoading,
    error,
    clientId,
    currentClient,

    fetchClients,
    fetchClientPage,
    createClient,
    updateClient,
    deleteClient,
  };
});
