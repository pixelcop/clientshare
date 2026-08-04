import axios from 'axios';
import { defineStore } from 'pinia';

const defaultPrimaryColor = '#3B82F6';
export const defaultBrandingTitle = 'ClientShare Portal';

function defaultBranding() {
  return {
    title: '',
    logo: '',
    primaryColor: defaultPrimaryColor,
    effectivePublicBaseURL: '',
    accountDashboardURL: '',
  };
}

export const useBrandingStore = defineStore('branding', () => {
  const branding = ref(defaultBranding());
  const loading = ref(false);
  const error = ref<string | null>(null);

  async function loadBranding() {
    loading.value = true;
    error.value = null;
    try {
      const res = await axios.get('/api/settings/branding');
      branding.value = {
        title: res.data.site_title || defaultBrandingTitle,
        logo: res.data.logo_path ? `/${res.data.logo_path}` : '',
        primaryColor: res.data.primary_color || defaultPrimaryColor,
        effectivePublicBaseURL: res.data.effective_public_base_url || '',
        accountDashboardURL: res.data.account_dashboard_url || '',
      };
    } catch {
      branding.value = {
        ...defaultBranding(),
        title: defaultBrandingTitle,
      };
      error.value = 'Failed to load branding';
    } finally {
      loading.value = false;
      document.documentElement.style.setProperty('--primary', branding.value.primaryColor);
      document.title = branding.value.title;
    }
  }

  loadBranding();

  return {
    branding,
    loadBranding,
  };
});
