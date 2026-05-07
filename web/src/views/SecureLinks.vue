<script setup lang="ts">
import axios from 'axios';
import { storeToRefs } from 'pinia';
import { Button } from 'primevue';

import SecureLinkModal from '@/components/SecureLinkModal.vue';
import { useSecureLinksStore } from '@/stores/secureLinks';
import { useTenantSettingsStore } from '@/stores/tenantSettings';
import type { Client, SecureLink } from '@/types/models';

const route = useRoute();
const linksStore = useSecureLinksStore();
const tenantSettingsStore = useTenantSettingsStore();
const { settings: tenantSettings } = storeToRefs(tenantSettingsStore);

const client = ref<Client | null>(null);
const clientLoading = ref(false);
const clientError = ref<string | null>(null);
const showCreate = ref(false);
const copiedLinkId = ref<string | null>(null);
const copyError = ref<string | null>(null);

const clientId = computed(() => route.params.id as string);

async function fetchClient() {
  clientLoading.value = true;
  clientError.value = null;
  try {
    const res = await axios.get(`/api/clients/${clientId.value}`);
    client.value = res.data;
  } catch (error: unknown) {
    if (axios.isAxiosError(error)) {
      clientError.value = error.response?.data?.error || 'Failed to load client';
    } else {
      clientError.value = 'Failed to load client';
    }
  } finally {
    clientLoading.value = false;
  }
}

async function load() {
  if (!clientId.value) {
    return;
  }
  await Promise.all([fetchClient(), linksStore.fetchLinks(clientId.value)]);
}

onMounted(load);
watch(() => route.params.id, load);

onMounted(() => {
  void tenantSettingsStore.loadTenantSettings().catch(() => undefined);
});

function linkUrl(link: SecureLink) {
  return link.link_url?.trim() || '';
}

function isExpired(link: SecureLink) {
  return new Date(link.expires_at).getTime() < Date.now();
}

function formatDate(dateStr?: string) {
  if (!dateStr) {
    return '-';
  }
  return new Date(dateStr).toLocaleString();
}

function formatAccess(accessType: string) {
  if (accessType === 'readwrite') {
    return 'Read & Write';
  }
  if (accessType === 'read') {
    return 'Read';
  }
  if (accessType === 'write') {
    return 'Write';
  }
  return accessType;
}

async function copyLink(e: PointerEvent, link: SecureLink) {
  e.preventDefault();
  copyError.value = null;
  const url = linkUrl(link);
  if (!url) {
    copyError.value = 'Secure link URL is unavailable. Refresh and try again.';
    return;
  }
  try {
    await navigator.clipboard.writeText(url);
    copiedLinkId.value = link.id;
    setTimeout(() => {
      if (copiedLinkId.value === link.id) {
        copiedLinkId.value = null;
      }
    }, 2000);
  } catch {
    // FIXME: show link to copy as a fallback
    copyError.value = 'Failed to copy link. Try again.';
  }
}

async function createLink(payload: {
  accessType: string;
  expiryDays: number;
  email: string;
  sendEmail: boolean;
}) {
  await linksStore.createLink(
    clientId.value,
    payload.accessType,
    payload.expiryDays,
    payload.sendEmail ? payload.email : '',
  );
  showCreate.value = false;
}

async function revokeLink(link: SecureLink) {
  const ok = window.confirm('Revoke this link? This action cannot be undone.');
  if (!ok) {
    return;
  }
  await linksStore.revokeLink(link.id);
}
</script>

<template>
  <div>
    <div class="flex items-center justify-between mb-4">
      <div>
        <h1 class="text-2xl font-bold">Secure Links</h1>
        <p v-if="client" class="text-sm text-gray-600">Manage links for {{ client.name }}</p>
      </div>
      <Button @click="showCreate = true">New Link</Button>
    </div>

    <div v-if="clientLoading" class="text-gray-600 mb-4">Loading client...</div>
    <div v-if="clientError" class="text-red-600 mb-4">{{ clientError }}</div>
    <div v-if="linksStore.error" class="text-red-600 mb-4">{{ linksStore.error }}</div>
    <div v-if="copyError" class="text-red-600 mb-4">{{ copyError }}</div>

    <table class="min-w-full bg-white rounded shadow">
      <thead>
        <tr>
          <th class="px-4 py-2 text-left">Link</th>
          <th class="px-4 py-2 text-left">Access</th>
          <th class="px-4 py-2 text-left">Email</th>
          <th class="px-4 py-2 text-left">Expires</th>
          <th class="px-4 py-2 text-left">Status</th>
          <th class="px-4 py-2 text-left">Last Accessed</th>
          <th class="px-4 py-2 text-left">Actions</th>
        </tr>
      </thead>
      <tbody>
        <tr v-if="!linksStore.loading && linksStore.links.length === 0">
          <td colspan="7" class="px-4 py-6 text-center text-gray-500">No links yet.</td>
        </tr>
        <tr v-for="link in linksStore.links" :key="link.id">
          <td class="px-4 py-2">
            <div class="flex flex-col gap-1">
              <a
                :href="linkUrl(link) || undefined"
                rel="noopener noreferrer"
                class="text-blue-600 hover:underline break-all"
                @click="(e) => copyLink(e, link)"
              >
                Generated {{ formatDate(link.created_at) }}
              </a>
            </div>
          </td>
          <td class="px-4 py-2">{{ formatAccess(link.access_type) }}</td>
          <td class="px-4 py-2">{{ link.email || '-' }}</td>
          <td class="px-4 py-2">{{ formatDate(link.expires_at) }}</td>
          <td class="px-4 py-2">
            <span
              :class="[
                'px-2 py-1 rounded text-xs font-semibold',
                isExpired(link) ? 'bg-red-100 text-red-700' : 'bg-green-100 text-green-700',
              ]"
            >
              {{ isExpired(link) ? 'Expired' : 'Active' }}
            </span>
          </td>
          <td class="px-4 py-2">{{ formatDate(link.last_accessed) }}</td>
          <td class="px-4 py-2 flex gap-2">
            <Button @click="(e) => copyLink(e, link)">
              {{ copiedLinkId === link.id ? 'Copied' : 'Copy' }}
            </Button>
            <Button label="Revoke" @click="revokeLink(link)" severity="danger" size="small" />
          </td>
        </tr>
      </tbody>
    </table>

    <SecureLinkModal
      :show="showCreate"
      :loading="linksStore.loading"
      :error="linksStore.error"
      :defaultExpiryDays="tenantSettings.secure_link_default_expiry_days"
      @close="showCreate = false"
      @submit="createLink"
    />
  </div>
</template>

<style scoped></style>
