import axios from 'axios';
import { defineStore } from 'pinia';

import type { TenantSettings } from '@/types/models';

const defaultSecureLinkExpiryDays = 90;

function defaultTenantSettings(): TenantSettings {
  return {
    id: '',
    tenant_id: '',
    site_title: '',
    logo_path: '',
    primary_color: '#3B82F6',
    public_base_url: '',
    effective_public_base_url: '',
    invite_welcome_text: '',
    secure_link_default_expiry_days: defaultSecureLinkExpiryDays,
    created_at: '',
    updated_at: '',
  };
}

export const useTenantSettingsStore = defineStore('tenant-settings', () => {
  const settings = ref<TenantSettings>(defaultTenantSettings());
  const loading = ref(false);
  const error = ref<string | null>(null);

  async function loadTenantSettings() {
    loading.value = true;
    error.value = null;
    try {
      const res = await axios.get('/api/settings/tenant');
      settings.value = {
        ...defaultTenantSettings(),
        ...res.data,
        secure_link_default_expiry_days:
          Number(res.data?.secure_link_default_expiry_days) || defaultSecureLinkExpiryDays,
      };
    } catch (err) {
      settings.value = defaultTenantSettings();
      error.value = axios.isAxiosError(err)
        ? err.response?.data?.error || 'Failed to load tenant settings'
        : 'Failed to load tenant settings';
      throw err;
    } finally {
      loading.value = false;
    }
  }

  return {
    settings,
    loading,
    error,
    loadTenantSettings,
  };
});
