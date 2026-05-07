import axios from 'axios';
import { defineStore } from 'pinia';

import type { PaginatedResult, PaginationResponse, User } from '@/types/models';
import { defaultPagination } from '@/types/util';

export interface UserCreatePayload {
  email: string;
  role: string;
  name: string;
  client_ids?: string[];
}

export interface UserUpdatePayload {
  email?: string;
  password?: string;
  role?: string;
  name?: string;
  client_ids?: string[];
}

export interface UserListFilters {
  role?: string;
  invite_status?: string;
  client_id?: string;
}

export interface GeneratedUserAccessLink {
  kind: 'invite' | 'password_reset';
  link: string;
  expires_at: string;
}

function normalizeUserListFilters(filters: UserListFilters = {}) {
  return {
    role: filters.role?.trim() ?? '',
    invite_status: filters.invite_status?.trim() ?? '',
    client_id: filters.client_id?.trim() ?? '',
  };
}

export const useUsersStore = defineStore('users', () => {
  const users = ref<User[]>([]);
  const loading = ref(false);
  const listedUsers = ref<User[]>([]);
  const listingPagination = ref<PaginationResponse>(defaultPagination());
  const listingSearch = ref('');
  const listingFilters = ref<UserListFilters>(normalizeUserListFilters());
  const listingLoading = ref(false);
  const error = ref<string | null>(null);

  function resolveErrorMessage(err: unknown, fallback: string) {
    if (axios.isAxiosError(err)) {
      return err.response?.data?.error || fallback;
    }
    if (err instanceof Error) {
      return err.message;
    }
    return fallback;
  }

  async function fetchUsers() {
    loading.value = true;
    error.value = null;
    try {
      const res = await axios.get('/api/users');
      users.value = res.data;
    } catch (err) {
      error.value = resolveErrorMessage(err, 'Failed to fetch users');
    } finally {
      loading.value = false;
    }
  }

  async function fetchUserPage(
    page = 1,
    pageSize = listingPagination.value.page_size,
    search = '',
    filters: UserListFilters = listingFilters.value,
  ) {
    listingLoading.value = true;
    error.value = null;
    listingSearch.value = search;
    listingFilters.value = normalizeUserListFilters(filters);
    try {
      const res = await axios.get<PaginatedResult<User>>('/api/users', {
        params: {
          page,
          page_size: pageSize,
          search,
          role: listingFilters.value.role,
          invite_status: listingFilters.value.invite_status,
          client_id: listingFilters.value.client_id,
        },
      });
      listedUsers.value = res.data.items || [];
      listingPagination.value = res.data.pagination || defaultPagination();
    } catch (err) {
      error.value = resolveErrorMessage(err, 'Failed to fetch users');
    } finally {
      listingLoading.value = false;
    }
  }

  async function refreshUserPage() {
    await fetchUserPage(
      listingPagination.value.page,
      listingPagination.value.page_size,
      listingSearch.value,
      listingFilters.value,
    );
  }

  async function createUser(payload: UserCreatePayload) {
    loading.value = true;
    error.value = null;
    try {
      const res = await axios.post('/api/users', payload);
      users.value.push(res.data);
      await refreshUserPage();
      return res.data;
    } catch (err) {
      error.value = resolveErrorMessage(err, 'Failed to create user');
      throw err;
    } finally {
      loading.value = false;
    }
  }

  async function updateUser(id: string, payload: UserUpdatePayload) {
    loading.value = true;
    error.value = null;
    try {
      const res = await axios.put(`/api/users/${id}`, payload);
      const idx = users.value.findIndex((u) => u.id === id);
      if (idx !== -1) {
        users.value[idx] = res.data;
      }
      await refreshUserPage();
      return res.data;
    } catch (err) {
      error.value = resolveErrorMessage(err, 'Failed to update user');
      throw err;
    } finally {
      loading.value = false;
    }
  }

  async function deleteUser(id: string) {
    loading.value = true;
    error.value = null;
    try {
      await axios.delete(`/api/users/${id}`);
      users.value = users.value.filter((u) => u.id !== id);
      await refreshUserPage();
    } catch (err) {
      error.value = resolveErrorMessage(err, 'Failed to delete user');
      throw err;
    } finally {
      loading.value = false;
    }
  }

  async function resendInvite(id: string) {
    loading.value = true;
    error.value = null;
    try {
      await axios.post(`/api/users/${id}/resend-invite`);
      await refreshUserPage();
    } catch (err) {
      error.value = resolveErrorMessage(err, 'Failed to resend invite');
      throw err;
    } finally {
      loading.value = false;
    }
  }

  async function generateAccessLink(id: string) {
    loading.value = true;
    error.value = null;
    try {
      const res = await axios.post<GeneratedUserAccessLink>(
        `/api/users/${id}/generate-access-link`,
      );
      return res.data;
    } catch (err) {
      error.value = resolveErrorMessage(err, 'Failed to generate access link');
      throw err;
    } finally {
      loading.value = false;
    }
  }

  async function resendAllInvites() {
    loading.value = true;
    error.value = null;
    try {
      const res = await axios.post<{ resent?: number }>('/api/users/resend-invites');
      await refreshUserPage();
      return res.data.resent ?? 0;
    } catch (err) {
      error.value = resolveErrorMessage(err, 'Failed to resend invites');
      throw err;
    } finally {
      loading.value = false;
    }
  }

  return {
    users,
    loading,
    listedUsers,
    listingPagination,
    listingSearch,
    listingFilters,
    listingLoading,
    error,
    fetchUsers,
    fetchUserPage,
    createUser,
    updateUser,
    deleteUser,
    resendInvite,
    generateAccessLink,
    resendAllInvites,
  };
});
