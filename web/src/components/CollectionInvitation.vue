<script setup lang="ts">
import { Button, Card } from 'openvue';

import { useBrandingStore } from '@/stores/branding';
import { guideTopic, useCollectionStore } from '@/stores/collection';

defineProps<{ clientId?: string }>();
const workflow = useCollectionStore();
const branding = useBrandingStore();
</script>

<template>
  <Card
    v-if="workflow.allowed && !workflow.progress"
    class="collection-invitation collection-theme"
  >
    <template #content>
      <div class="collection-invite-layout">
        <span class="collection-invite-icon" aria-hidden="true"
          ><i class="pi pi-file-import"
        /></span>
        <div class="collection-invite-copy">
          <h2>Collect your first client document</h2>
          <p>Create an upload link your client can use without an account.</p>
        </div>
      </div>
      <div class="collection-invite-actions">
        <Button label="Collect a document" @click="workflow.start(clientId)" />
        <a
          :href="
            guideTopic(
              workflow.guideBase || branding.branding.docsURL,
              'getting-started/first-client-exchange',
            )
          "
          target="_blank"
          rel="noopener noreferrer"
          class="collection-invite-guide"
          >Read the guide</a
        >
      </div>
    </template>
  </Card>
</template>

<style scoped>
.collection-invitation {
  margin-block: 16px;
  background: var(--collection-desk);
  box-shadow: none;
  border: 1px solid var(--collection-rule);
  border-radius: 12px;
}
.collection-invitation :deep(.p-card-body) {
  padding: 24px;
}
.collection-invite-layout {
  display: flex;
  gap: 16px;
  align-items: flex-start;
}
.collection-invite-icon {
  display: grid;
  place-items: center;
  width: 48px;
  height: 56px;
  flex-shrink: 0;
  border: 1px solid var(--collection-rule);
  background: var(--collection-paper);
  color: var(--p-primary-color);
  border-radius: 8px;
}
.collection-invite-icon .pi {
  font-size: 24px;
}
.collection-invite-copy h2 {
  font-size: 18px;
  line-height: 1.3;
  font-weight: 650;
  letter-spacing: -0.02em;
  margin: 4px 0 8px;
  text-wrap: balance;
}
.collection-invite-copy p {
  font-size: 14px;
  line-height: 1.5;
  color: var(--collection-secondary);
  margin: 0 0 8px;
  text-wrap: pretty;
}
.collection-invite-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 16px;
  margin-top: 20px;
}
.collection-invite-actions .p-button {
  min-height: 44px;
}
.collection-invite-guide {
  color: var(--collection-secondary);
  font-size: 13px;
  display: inline-flex;
  align-items: center;
  min-height: 44px;
}
.collection-invite-guide:hover {
  text-decoration: underline;
}
@media (max-width: 600px) {
  .collection-invitation :deep(.p-card-body) {
    padding: 16px;
  }
  .collection-invite-icon {
    display: none;
  }
  .collection-invite-copy h2 {
    font-size: 16px;
  }
  .collection-invite-copy p {
    font-size: 13px;
  }
  .collection-invite-actions {
    margin-top: 12px;
    gap: 4px 12px;
  }
}
</style>
