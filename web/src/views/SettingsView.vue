<script setup lang="ts">
import axios from 'axios';
import { storeToRefs } from 'pinia';
import { Button, FileUpload, type FileUploadSelectEvent, Select, useToast } from 'primevue';

import Logo from '@/components/Logo.vue';
import { defaultBrandingTitle, useBrandingStore } from '@/stores/branding';
import { useTenantSettingsStore } from '@/stores/tenantSettings';
import type { QueuedEmail } from '@/types/models';

import { formatDate } from '../types/util';

const brandingStore = useBrandingStore();
const tenantSettingsStore = useTenantSettingsStore();
const { branding } = storeToRefs(brandingStore);
const { settings: tenantSettings } = storeToRefs(tenantSettingsStore);
const toast = useToast();

type LogLevelOption = {
  label: string;
  value: string;
};

type SystemSettings = {
  tenancy_mode?: string;
  log_level?: {
    level?: string;
    options?: string[];
  };
};

const fallbackLogLevelOptions = ['debug', 'info', 'warn', 'error', 'dpanic', 'panic', 'fatal'];
const defaultPrimaryColor = '#3B82F6';
const defaultSecureLinkExpiryDays = 90;

const form = reactive({
  siteTitle: '',
  primaryColor: defaultPrimaryColor,
  publicBaseURL: '',
  inviteWelcomeText: '',
  secureLinkDefaultExpiryDays: defaultSecureLinkExpiryDays,
  logLevel: 'info',
});
const logoFile = ref<File | null>(null);
const logoPreview = ref<string>('');
const removeLogo = ref(false);
const loading = ref(false);
const saving = ref(false);
const tenancyMode = ref('single');
const pendingEmails = ref<QueuedEmail[]>([]);
const emailQueueLoading = ref(false);
const emailQueueError = ref('');
const emailQueueProcessing = ref(false);
const emailQueueSuccess = ref('');

const effectivePublicBaseURL = computed(() => {
  return (
    tenantSettings.value.effective_public_base_url || branding.value.effectivePublicBaseURL || ''
  );
});

const isSingleTenant = computed(() => tenancyMode.value !== 'hosted');

function buildLogLevelOptions(levels: string[]): LogLevelOption[] {
  return levels.map((level) => ({
    label: level.toUpperCase(),
    value: level,
  }));
}

const logLevelOptions = ref<LogLevelOption[]>(buildLogLevelOptions(fallbackLogLevelOptions));

function getAxiosErrorMessage(err: unknown, fallback: string) {
  if (axios.isAxiosError(err)) {
    return err.response?.data?.error || fallback;
  }
  return fallback;
}

function publicAssetPath(path: string) {
  const trimmed = path.trim().replace(/^\/+/, '');
  return trimmed ? `/${trimmed}` : '';
}

function applyTenantSettingsToForm() {
  form.siteTitle = tenantSettings.value.site_title || '';
  form.primaryColor = tenantSettings.value.primary_color || defaultPrimaryColor;
  form.publicBaseURL = tenantSettings.value.public_base_url || '';
  form.inviteWelcomeText = tenantSettings.value.invite_welcome_text || '';
  form.secureLinkDefaultExpiryDays =
    tenantSettings.value.secure_link_default_expiry_days || defaultSecureLinkExpiryDays;
  logoPreview.value = publicAssetPath(tenantSettings.value.logo_path || '');
  removeLogo.value = false;
  logoFile.value = null;
}

async function fetchTenantSettings() {
  loading.value = true;
  try {
    await Promise.all([brandingStore.loadBranding(), tenantSettingsStore.loadTenantSettings()]);
    applyTenantSettingsToForm();
  } catch {
    toast.add({
      severity: 'error',
      detail: 'Failed to load tenant settings. Try refreshing the page.',
      life: 5000,
    });
  } finally {
    loading.value = false;
  }
}

async function fetchSystemSettings() {
  try {
    const res = await axios.get<SystemSettings>('/api/settings/system');
    tenancyMode.value = res.data?.tenancy_mode || 'single';

    if (res.data?.log_level) {
      form.logLevel = res.data.log_level.level || 'info';
      const options = Array.isArray(res.data.log_level.options)
        ? res.data.log_level.options
        : fallbackLogLevelOptions;
      logLevelOptions.value = buildLogLevelOptions(options);
      return;
    }

    logLevelOptions.value = buildLogLevelOptions(fallbackLogLevelOptions);
  } catch {
    tenancyMode.value = 'single';
    logLevelOptions.value = buildLogLevelOptions(fallbackLogLevelOptions);
  }
}

function onLogoChange(event: FileUploadSelectEvent) {
  const file = event.files?.pop() || null;
  logoFile.value = file;
  removeLogo.value = false;
  if (!file) {
    logoPreview.value = publicAssetPath(tenantSettings.value.logo_path || '');
    return;
  }
  const reader = new FileReader();
  reader.onload = () => {
    logoPreview.value = String(reader.result || '');
  };
  reader.readAsDataURL(file);
}

function onRemoveLogoChange() {
  if (removeLogo.value) {
    logoFile.value = null;
    logoPreview.value = '';
  } else {
    logoPreview.value = publicAssetPath(tenantSettings.value.logo_path || '');
  }
}

function normalizeSecureLinkExpiryDays() {
  const parsed = Number(form.secureLinkDefaultExpiryDays);
  if (!Number.isFinite(parsed)) {
    return defaultSecureLinkExpiryDays;
  }
  return Math.min(Math.max(Math.round(parsed), 1), 120);
}

async function saveSettings() {
  saving.value = true;
  const formData = new FormData();
  formData.append('site_title', form.siteTitle.trim());
  formData.append('primary_color', form.primaryColor.trim());
  formData.append('public_base_url', form.publicBaseURL.trim());
  formData.append('invite_welcome_text', form.inviteWelcomeText.trim());
  formData.append('secure_link_default_expiry_days', String(normalizeSecureLinkExpiryDays()));
  formData.append('remove_logo', removeLogo.value ? 'true' : 'false');
  if (logoFile.value) {
    formData.append('logo', logoFile.value);
  }

  let settingsSaved = false;

  try {
    await axios.put('/api/settings/tenant', formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    });
    settingsSaved = true;
    toast.add({
      severity: 'success',
      summary: 'Settings updated',
      life: 3000,
    });
  } catch {
    toast.add({
      severity: 'error',
      detail: 'Failed to save settings. Try again.',
      life: 5000,
    });
  }

  if (isSingleTenant.value) {
    try {
      const res = await axios.put<SystemSettings>('/api/settings/system', { level: form.logLevel });
      if (res.data?.log_level?.level) {
        form.logLevel = res.data.log_level.level;
      }
      const options = Array.isArray(res.data?.log_level?.options)
        ? res.data.log_level.options
        : fallbackLogLevelOptions;
      logLevelOptions.value = buildLogLevelOptions(options);
    } catch {
      // Leave the tenant settings save result intact if the runtime log-level update fails.
    }
  }

  try {
    if (settingsSaved) {
      await Promise.all([brandingStore.loadBranding(), tenantSettingsStore.loadTenantSettings()]);
      applyTenantSettingsToForm();
    }
  } finally {
    saving.value = false;
  }
}

async function fetchPendingEmails() {
  emailQueueLoading.value = true;
  emailQueueError.value = '';
  emailQueueSuccess.value = '';
  try {
    const res = await axios.get('/api/email/queue');
    pendingEmails.value = res.data?.pending ?? [];
  } catch (err) {
    emailQueueError.value = getAxiosErrorMessage(err, 'Failed to load email queue.');
  } finally {
    emailQueueLoading.value = false;
  }
}

async function processEmailQueue() {
  emailQueueProcessing.value = true;
  emailQueueError.value = '';
  emailQueueSuccess.value = '';
  try {
    await axios.post('/api/email/queue/process');
    emailQueueSuccess.value = 'Queued emails sent.';
    await fetchPendingEmails();
  } catch (err) {
    emailQueueError.value = getAxiosErrorMessage(err, 'Failed to process email queue.');
  } finally {
    emailQueueProcessing.value = false;
  }
}

watch(
  () => form.primaryColor,
  (value) => {
    if (value) {
      if (!value.startsWith('#')) {
        value = `#${value}`;
        form.primaryColor = value;
      }
      document.documentElement.style.setProperty('--primary', value);
    }
  },
);

onMounted(() => {
  void fetchTenantSettings();
  void fetchSystemSettings();
  void fetchPendingEmails();
});
</script>

<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold">Settings</h1>
        <p class="text-sm text-muted-color">
          Manage tenant branding, invites, links, and appearance.
        </p>
      </div>
      <Button :disabled="saving || loading" @click="saveSettings" severity="primary">
        {{ saving ? 'Saving...' : 'Save Changes' }}
      </Button>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <Card class="lg:col-span-2 rounded shadow">
        <template #title>
          <h2 class="text-lg font-semibold">Branding</h2>
        </template>

        <template #content>
          <div class="flex flex-col space-y-4">
            <div class="flex flex-col gap-2">
              <label class="text-sm font-medium">Site title</label>
              <InputText
                v-model="form.siteTitle"
                type="text"
                class="w-full border rounded px-3 py-2"
                placeholder="ClientShare Portal"
              />
            </div>

            <div class="flex flex-col gap-2">
              <label class="text-md font-bold">Logo upload</label>
              <div class="flex flex-row gap-4">
                <div v-if="logoPreview || removeLogo" class="flex flex-col items-center">
                  <h6>Current Logo</h6>
                  <img
                    v-if="logoPreview"
                    :src="logoPreview"
                    alt="Logo preview"
                    class="h-10 w-10 object-contain rounded mb-4"
                  />
                  <i v-else class="pi pi-minus h-10 w-10 mb-4 text-center pt-4"></i>

                  <label class="inline-flex items-center gap-2 text-sm">
                    <input v-model="removeLogo" type="checkbox" @change="onRemoveLogoChange" />
                    Remove current logo
                  </label>
                </div>
                <FileUpload
                  class="p-button-outlined"
                  ref="fileupload"
                  mode="advanced"
                  name="logo[]"
                  :multiple="false"
                  :show-upload-button="false"
                  :show-cancel-button="false"
                  custom-upload
                  severity="info"
                  accept="image/*"
                  :maxFileSize="1000000"
                  @select="onLogoChange"
                >
                  <template #content>
                    <span>Drag and drop image here to upload.</span>
                  </template>
                </FileUpload>
              </div>
            </div>

            <!-- FIXME: when we properly set colors -->
            <div v-if="false" class="flex flex-col gap-2">
              <label class="text-sm font-medium">Primary color</label>
              <div class="flex items-center gap-3">
                <ColorPicker
                  v-model="form.primaryColor"
                  type="color"
                  class="picker h-10 w-16 rounded border"
                />
                <InputText
                  v-model="form.primaryColor"
                  type="text"
                  class="flex-1 border rounded px-3 py-2"
                  placeholder="#3B82F6"
                />
              </div>
            </div>

            <div v-if="isSingleTenant" class="flex flex-col gap-2">
              <label class="text-sm font-medium">Public base URL</label>
              <InputText
                v-model="form.publicBaseURL"
                type="url"
                class="w-full border rounded px-3 py-2"
                placeholder="https://portal.example.com"
              />
              <p class="text-xs text-muted-color">
                Used for invite, reset-password, and secure-link URLs for this tenant.
              </p>
              <p v-if="effectivePublicBaseURL" class="text-xs text-muted-color">
                Current effective URL: {{ effectivePublicBaseURL }}
              </p>
            </div>

            <div class="flex flex-col gap-2">
              <label class="text-sm font-medium">Invite welcome text</label>
              <textarea
                v-model="form.inviteWelcomeText"
                rows="4"
                class="w-full rounded border px-3 py-2"
                placeholder="Welcome to your client portal."
              />
            </div>

            <div class="flex flex-col gap-2">
              <label class="text-sm font-medium">Default secure-link expiry</label>
              <input
                v-model.number="form.secureLinkDefaultExpiryDays"
                type="number"
                min="1"
                max="120"
                class="w-full rounded border px-3 py-2"
              />
              <p class="text-xs text-muted-color">
                Applied when creating secure links unless the user overrides it.
              </p>
            </div>

            <div v-if="isSingleTenant" class="flex flex-col gap-2">
              <label class="text-sm font-medium">Runtime log level</label>
              <Select
                v-model="form.logLevel"
                :options="logLevelOptions"
                optionLabel="label"
                optionValue="value"
                class="w-full"
                placeholder="Select a log level"
              />
              <p class="text-xs text-muted-color">
                Applies immediately and resets when the server restarts.
              </p>
            </div>
          </div>
        </template>
      </Card>

      <Card class="rounded shadow p-6 space-y-4">
        <template #title>
          <h2 class="text-lg font-semibold">Preview</h2>
        </template>

        <template #content>
          <div class="rounded border p-4 space-y-3">
            <div class="gap-3">
              <p class="text-sm mb-2">Header</p>
              <div class="flex layout-topbar-logo-container">
                <div class="layout-topbar-logo">
                  <img
                    v-if="logoPreview"
                    :src="logoPreview"
                    alt="Logo preview"
                    class="h-10 w-10 object-contain rounded"
                  />
                  <span v-if="form.siteTitle && form.siteTitle !== defaultBrandingTitle">{{
                    form.siteTitle
                  }}</span>
                  <Logo v-else />
                </div>
              </div>
            </div>
            <div class="rounded border p-3 space-y-1 text-sm text-slate-600">
              <p>
                <span class="font-medium text-slate-900">Invite copy:</span>
                {{ form.inviteWelcomeText || 'Default invite email copy will be used.' }}
              </p>
              <p>
                <span class="font-medium text-slate-900">Default link expiry:</span>
                {{ normalizeSecureLinkExpiryDays() }} days
              </p>
            </div>
            <!-- FIXME: when we properly set colors -->
            <div v-if="false" class="rounded border p-3 space-y-2">
              <p class="text-sm">Buttons</p>
              <Button :style="{ backgroundColor: form.primaryColor || defaultPrimaryColor }">
                Primary action
              </Button>
            </div>
          </div>
        </template>
      </Card>
    </div>

    <Card class="rounded shadow space-y-4">
      <template #title>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 class="text-lg font-semibold mb-0!">Email notifications</h2>
            <p class="text-sm text-muted-color">Pending queued emails waiting to be sent.</p>
          </div>
          <div class="flex items-center gap-2">
            <Button
              :disabled="emailQueueLoading || emailQueueProcessing"
              @click="fetchPendingEmails"
              severity="secondary"
            >
              Refresh
            </Button>
            <Button
              :disabled="emailQueueLoading || emailQueueProcessing"
              @click="processEmailQueue"
              severity="info"
            >
              {{ emailQueueProcessing ? 'Sending...' : 'Send now' }}
            </Button>
          </div>
        </div>
      </template>

      <template #content>
        <div
          v-if="emailQueueError"
          class="rounded border border-red-200 bg-red-50 text-red-700 px-4 py-2"
        >
          {{ emailQueueError }}
        </div>
        <div
          v-if="emailQueueSuccess"
          class="rounded border border-green-200 bg-green-50 text-green-700 px-4 py-2"
        >
          {{ emailQueueSuccess }}
        </div>

        <div v-if="emailQueueLoading" class="text-sm">Loading queued emails...</div>
        <div v-else>
          <div v-if="pendingEmails.length === 0" class="text-sm mt-4">
            No pending email notifications.
          </div>
          <table v-else class="min-w-full border rounded overflow-hidden">
            <thead class="text-left text-sm">
              <tr>
                <th class="px-4 py-2">Recipient</th>
                <th class="px-4 py-2">Subject</th>
                <th class="px-4 py-2">Scheduled</th>
                <th class="px-4 py-2">Queued</th>
              </tr>
            </thead>
            <tbody class="text-sm">
              <tr v-for="email in pendingEmails" :key="email.id" class="border-t">
                <td class="px-4 py-2">{{ email.recipient }}</td>
                <td class="px-4 py-2">{{ email.subject }}</td>
                <td class="px-4 py-2">{{ formatDate(email.scheduled_for) }}</td>
                <td class="px-4 py-2">{{ formatDate(email.created_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
    </Card>
  </div>
</template>

<style scoped>
.picker {
  --p-colorpicker-preview-height: 100%;
  --p-colorpicker-preview-width: 100%;
}
</style>
