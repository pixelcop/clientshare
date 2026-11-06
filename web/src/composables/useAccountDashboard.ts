import { useAuthStore } from '@/stores/auth';
import { useBrandingStore } from '@/stores/branding';

// Match the account navigation: only the workspace owner can manage billing,
// and only installations configured with a hosted account origin have a URL.
export function useAccountDashboard() {
  const auth = useAuthStore();
  const branding = useBrandingStore();

  return computed(() => {
    if (!auth.user?.is_initial_admin || !branding.branding.accountDashboardURL) {
      return '';
    }
    const url = new URL(branding.branding.accountDashboardURL);
    // Keep the email hint out of HTTP requests and Referer headers.
    url.hash = `email=${encodeURIComponent(auth.user.email)}`;
    return url.toString();
  });
}
