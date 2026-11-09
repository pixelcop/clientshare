<script setup lang="ts">
import axios from 'axios';
import { Button } from 'openvue';
import { storeToRefs } from 'pinia';

import SecureLinkModal from '@/components/SecureLinkModal.vue';
import { useCollectionStore } from '@/stores/collection';
import { useSecureLinksStore } from '@/stores/secureLinks';
import { useTenantSettingsStore } from '@/stores/tenantSettings';
import type { LinkAccessMode } from '@/types/linkAccess';
import { formatLinkExpiry, isUsableLink, linkAccessLabel } from '@/types/linkAccess';
import type { Client, SecureLink } from '@/types/models';

const route = useRoute();
const router = useRouter();
const collection = useCollectionStore();
const linksStore = useSecureLinksStore();
const tenantSettingsStore = useTenantSettingsStore();
const { settings: tenantSettings } = storeToRefs(tenantSettingsStore);
const client = ref<Client | null>(null);
const clientLoading = ref(false);
const clientError = ref<string | null>(null);
const showCreate = ref(false);
const result = ref<SecureLink | null>(null);
const showHistory = ref(false);
const copiedLinkId = ref<string | null>(null);
const copyError = ref<string | null>(null);
const now = ref(Date.now());
const clientId = computed(() => route.params.id as string);
let loadVersion = 0;
let clock: ReturnType<typeof setInterval> | undefined;
const usableLinks = computed(() =>
  linksStore.links.filter((link) => isUsableLink(link, now.value)),
);
const historyLinks = computed(() =>
  linksStore.links.filter((link) => !isUsableLink(link, now.value)),
);
const visibleLinks = computed(() => (showHistory.value ? historyLinks.value : usableLinks.value));

function openCreate() {
  result.value = null;
  showCreate.value = true;
}

async function load() {
  loadVersion += 1;
  const version = loadVersion;
  const id = clientId.value;
  client.value = null;
  result.value = null;
  showCreate.value = false;
  showHistory.value = false;
  clientLoading.value = true;
  clientError.value = null;
  try {
    const [response] = await Promise.all([
      axios.get<Client>(`/api/clients/${id}`),
      linksStore.fetchLinks(id),
    ]);
    if (version !== loadVersion) {
      return;
    }
    client.value = response.data;
    if (collection.guidance && !collection.progress?.clientId) {
      collection.selectClient(id);
    }
    if (typeof route.query.created === 'string') {
      result.value = linksStore.links.find((link) => link.id === route.query.created) || null;
      showCreate.value = !!result.value;
    }
  } catch (error: unknown) {
    if (version === loadVersion) {
      clientError.value =
        (axios.isAxiosError(error) && error.response?.data?.error) || 'Failed to load client';
    }
  } finally {
    if (version === loadVersion) {
      clientLoading.value = false;
    }
  }
}

watch(clientId, load, { immediate: true });
watch(
  () => route.query.collect,
  async (action) => {
    if (action !== 'create-link' || !collection.allowed) {
      return;
    }
    await tenantSettingsStore.loadTenantSettings().catch(() => undefined);
    if (route.query.collect !== action) {
      return;
    }
    openCreate();
    const query = { ...route.query };
    delete query.collect;
    void router.replace({ query });
  },
  { immediate: true },
);
onMounted(() => {
  void tenantSettingsStore.loadTenantSettings().catch(() => undefined);
  clock = setInterval(() => {
    now.value = Date.now();
  }, 1000);
});
onBeforeUnmount(() => {
  if (clock) {
    clearInterval(clock);
  }
});

async function copyLink(link: SecureLink) {
  copyError.value = null;
  if (!isUsableLink(link, Date.now()) || !link.link_url) {
    copyError.value = 'This link is no longer available.';
    return;
  }
  try {
    await navigator.clipboard.writeText(link.link_url);
    copiedLinkId.value = link.id;
    if (['write', 'readwrite'].includes(link.access_type)) {
      collection.shared(clientId.value, link.id, 'copied');
    }
  } catch {
    copyError.value = 'Failed to copy. Open link details to select the address.';
  }
}

async function createLink(payload: {
  accessType: LinkAccessMode;
  expiryDays: number;
  expiresAt: string;
}) {
  const id = clientId.value;
  try {
    const created = await linksStore.createLink(
      id,
      payload.accessType,
      payload.expiryDays,
      '',
      payload.expiresAt,
    );
    if (clientId.value === id) {
      result.value = created;
    }
  } catch {
    /* The creation form keeps its settings, error and retry key. */
  }
}

async function revokeLink(link: SecureLink) {
  if (
    !window.confirm(
      'Revoke this client-wide link? Anyone using it will lose access through this link. Other links and account access remain unchanged.',
    )
  ) {
    return;
  }
  try {
    await linksStore.revokeLink(link.id);
    now.value = Date.now();
    if (result.value?.id === link.id) {
      result.value = linksStore.links.find((item) => item.id === link.id) || null;
    }
  } catch {
    /* The list displays the server error without pretending revocation succeeded. */
  }
}
</script>

<template>
  <div class="secure-links">
    <header class="links-header">
      <div>
        <h1>Client-wide links</h1>
        <p v-if="client">{{ client.name }} · Entire client workspace</p>
      </div>
      <Button
        id="collect-new-link"
        label="New link"
        :disabled="!client || linksStore.loading"
        @click="openCreate"
      />
    </header>
    <p class="links-note">
      Anyone with a link gets its permissions. Removing someone’s account access won’t stop them
      using a link they already have.
    </p>
    <p v-if="clientLoading" role="status">Loading client…</p>
    <p v-if="clientError" role="alert" class="links-error">{{ clientError }}</p>
    <p v-if="linksStore.error" role="alert" class="links-error">{{ linksStore.error }}</p>
    <p v-if="copyError" role="alert" class="links-error">{{ copyError }}</p>
    <div class="links-tabs" aria-label="Link visibility">
      <Button
        :label="`Usable links (${usableLinks.length})`"
        :aria-pressed="!showHistory"
        :severity="!showHistory ? undefined : 'secondary'"
        @click="showHistory = false"
      />
      <Button
        :label="`Expired & revoked history (${historyLinks.length})`"
        :aria-pressed="showHistory"
        :severity="showHistory ? undefined : 'secondary'"
        @click="showHistory = true"
      />
    </div>
    <p v-if="linksStore.loading" role="status">Loading links…</p>
    <div v-else-if="!visibleLinks.length && !linksStore.error" class="links-empty">
      {{
        showHistory
          ? 'No expired or revoked links in history.'
          : 'No usable links. Create a link to share this client workspace.'
      }}
    </div>
    <div v-else class="links-table-wrapper">
      <table>
        <thead>
          <tr>
            <th>Created</th>
            <th>Permissions</th>
            <th>Expires</th>
            <th>Status</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="link in visibleLinks" :key="link.id">
            <td>{{ new Date(link.created_at).toLocaleString() }}</td>
            <td>{{ linkAccessLabel(link.access_type) }}</td>
            <td>{{ formatLinkExpiry(link.expires_at) }}</td>
            <td>
              {{ link.revoked_at ? 'Revoked' : isUsableLink(link, now) ? 'Usable' : 'Expired'
              }}<span v-if="link.revoked_at" class="block text-xs">{{
                formatLinkExpiry(link.revoked_at)
              }}</span>
            </td>
            <td>
              <div v-if="isUsableLink(link, now)" class="links-row-actions">
                <Button
                  :id="`collect-copy-${link.id}`"
                  :label="copiedLinkId === link.id ? 'Copied' : 'Copy'"
                  severity="secondary"
                  @click="copyLink(link)"
                />
                <Button
                  label="Details & email"
                  severity="secondary"
                  @click="
                    result = link;
                    showCreate = true;
                  "
                />
                <Button label="Revoke" severity="danger" variant="text" @click="revokeLink(link)" />
              </div>
              <span v-else class="links-note">Unavailable</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <SecureLinkModal
      :capabilities="client?.capabilities"
      :key="clientId"
      :show="showCreate"
      :result
      :targetName="client?.name"
      :collection="collection.progress?.clientId === clientId"
      :loading="linksStore.loading"
      :error="linksStore.error"
      :defaultExpiryDays="tenantSettings.secure_link_default_expiry_days"
      @close="showCreate = false"
      @submit="createLink"
    />
  </div>
</template>

<style scoped>
.secure-links {
  display: grid;
  gap: 16px;
  font-size: 14px;
}
.links-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
}
.links-header h1 {
  font-size: 28px;
  font-weight: 600;
}
.links-header p,
.links-note {
  color: var(--p-text-muted-color);
  font-size: 12px;
}
.links-tabs,
.links-row-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.secure-links :deep(.p-button) {
  min-height: 44px;
}
.links-table-wrapper {
  overflow-x: auto;
  border: 1px solid var(--p-content-border-color);
  border-radius: 8px;
  background: var(--p-content-background);
}
table {
  width: 100%;
  border-collapse: collapse;
}
th,
td {
  text-align: left;
  padding: 12px 16px;
  border-bottom: 1px solid var(--p-content-border-color);
  vertical-align: top;
}
th {
  font-size: 12px;
  font-weight: 500;
  color: var(--p-text-muted-color);
}
td {
  min-width: 140px;
  font-variant-numeric: tabular-nums;
}
.links-empty {
  padding: 32px 16px;
  border: 1px solid var(--p-content-border-color);
  border-radius: 8px;
  background: var(--p-content-background);
  text-align: center;
}
.links-error {
  color: var(--p-red-600);
}
</style>
