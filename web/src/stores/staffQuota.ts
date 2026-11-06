import axios from 'axios';
import { defineStore } from 'pinia';

interface StaffUsage {
  staff_users: number;
  max_users: number;
}

export const useStaffQuotaStore = defineStore('staffQuota', () => {
  const usage = ref<StaffUsage | null>(null);
  const loading = ref(false);
  const error = ref('');
  const atLimit = computed(
    () =>
      !!usage.value &&
      usage.value.max_users > 0 &&
      usage.value.staff_users >= usage.value.max_users,
  );
  const canAddStaff = computed(
    () => !!usage.value && !loading.value && !error.value && !atLimit.value,
  );
  let pending: Promise<void> | null = null;

  function refresh(): Promise<void> {
    if (pending) {return pending;}
    loading.value = true;
    error.value = '';
    pending = axios
      .get<StaffUsage>('/api/settings/usage')
      .then(({ data }) => {
        usage.value = data;
      })
      .catch(() => {
        error.value = 'Unable to check staff seats. Try again before inviting staff.';
      })
      .finally(() => {
        loading.value = false;
        pending = null;
      });
    return pending;
  }

  return { usage, loading, error, atLimit, canAddStaff, refresh };
});
