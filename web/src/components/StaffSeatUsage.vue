<script setup lang="ts">
import { useAccountDashboard } from '@/composables/useAccountDashboard';
import { useStaffQuotaStore } from '@/stores/staffQuota';

const quota = useStaffQuotaStore();
const accountDashboardURL = useAccountDashboard();
</script>

<template>
  <div class="mb-4 text-sm text-muted-color" role="status">
    <p v-if="quota.loading">Checking staff seats…</p>
    <template v-else-if="quota.error">
      <p>{{ quota.error }}</p>
      <Button type="button" severity="secondary" variant="text" @click="quota.refresh()">
        Retry
      </Button>
    </template>
    <template v-else-if="quota.usage">
      <p>
        Staff seats: {{ quota.usage.staff_users }} of {{ quota.usage.max_users || 'unlimited' }}
        (including pending invitations).
      </p>
      <template v-if="quota.atLimit">
        <p>
          All staff seats are in use. Free a seat to invite more staff. Customer users are
          unlimited.
        </p>
        <a
          v-if="accountDashboardURL"
          :href="accountDashboardURL"
          target="_blank"
          rel="noopener"
          class="text-primary underline"
          >Upgrade your plan in Account &amp; billing</a
        >
      </template>
    </template>
  </div>
</template>
