<script setup lang="ts">
import '@/assets/collection.css';
import '@openvue/openicons/openicons.css';

import { useMediaQuery } from '@vueuse/core';
import axios from 'axios';
import { Button, Message, Panel, Step, StepItem, StepPanel, Stepper } from 'openvue';

import CollectionSpotlight from '@/components/CollectionSpotlight.vue';
import { useAuthStore } from '@/stores/auth';
import { useBrandingStore } from '@/stores/branding';
import { useClientsStore } from '@/stores/clients';
import { guideTopic, useCollectionStore } from '@/stores/collection';
import { useSecureLinksStore } from '@/stores/secureLinks';

const workflow = useCollectionStore();
const clients = useClientsStore();
const auth = useAuthStore();
const branding = useBrandingStore();
const route = useRoute();
const tucked = ref(false);
const dockAtTop = ref(false);
const mobile = useMediaQuery('(max-width: 600px)');
const showAllSteps = ref(false);
const titles = [
  'Choose or create a client',
  'Create an upload link',
  'Copy and share the link',
  'Check for the document',
];
const topics = [
  'clients/manage-clients',
  'sharing/create-link',
  'sharing/request-documents',
  'activity/review-activity',
];
const guide = computed(() =>
  guideTopic(workflow.guideBase || branding.branding.docsURL, topics[Math.min(workflow.step, 3)]!),
);
const current = computed(() => Math.min(workflow.step, 3));
const nextAction = computed(() => {
  const clientId = workflow.progress?.clientId;
  const directory = route.path === '/clients';
  const workspace = route.path.startsWith('/folder/') && clients.clientId === clientId;
  const links = clientId && route.path === `/clients/${clientId}/links`;
  if (workflow.step === 0 && directory) {
    return clients.clients.length
      ? {
          targets: ['collect-client-search'],
          title: 'Choose a client',
          body: 'Search for your client, then select their name. To add one, select New Client.',
        }
      : {
          targets: ['collect-new-client'],
          title: 'Create a client',
          body: 'Select New Client, then enter their name.',
        };
  }
  if (workflow.step > 0 && directory) {
    const visible = clients.listedClients.some((client) => client.id === clientId);
    return visible
      ? {
          targets: [`collect-client-${clientId}`],
          title: 'Open the client workspace',
          body: `Select ${workflow.client?.name || 'the client name'} to open their files and sharing controls.`,
        }
      : {
          targets: ['collect-client-search'],
          title: 'Find your client',
          body: `Search for ${workflow.client?.name || 'your client'}, then select their name.`,
        };
  }
  if (workspace && workflow.step < 3) {
    return {
      targets: [`collect-links-${clientId}`],
      title: 'Open Secure Links',
      body: 'Select Secure Links to manage this client’s upload links.',
    };
  }
  if (links && workflow.step === 1) {
    return {
      targets: ['collect-new-link'],
      title: 'Create an upload link',
      body: 'Select New Link to choose upload access and an expiry date.',
    };
  }
  if (links && workflow.step === 2) {
    return {
      targets: [`collect-copy-${workflow.link?.id}`],
      title: 'Copy the upload link',
      body: 'Select Copy, then paste the link into your message. Copying does not send it.',
    };
  }
  if (workspace && workflow.step >= 3) {
    return null;
  }
  return {
    targets: ['collect-nav-clients', 'collect-nav-menu'],
    title: 'Go to Clients',
    body: 'Select Clients in the navigation. On mobile, open the menu first.',
  };
});
const spotlight = ref<{ targets: string[]; title: string; body: string } | null>(null);
const panelCollapsed = computed(
  () => workflow.progress?.collapsed || tucked.value || (mobile.value && !!spotlight.value),
);
const hintUnavailable = ref(false);
let expiryTimer: ReturnType<typeof setTimeout> | undefined;
function clearHint() {
  spotlight.value = null;
  workflow.cue = null;
  hintUnavailable.value = false;
}
function unavailable() {
  clearHint();
  hintUnavailable.value = true;
}
function minimize(collapsed: boolean) {
  tucked.value = false;
  dockAtTop.value = false;
  clearHint();
  workflow.stopGuidance();
  workflow.save({ collapsed });
}
function dismiss() {
  clearHint();
  workflow.stopGuidance();
  workflow.save({ dismissed: true });
}
function highlightStep(index: number) {
  if (index !== current.value || workflow.loading) {
    return;
  }
  tucked.value = false;
  dockAtTop.value = false;
  workflow.guide('start');
  if (!nextAction.value && workflow.step >= 3) {
    spotlight.value = {
      targets: ['collect-files'],
      title: 'Check the client’s files',
      body: 'Uploaded documents appear here. Reload the page to check for new files.',
    };
    workflow.cue = null;
  }
}
watch(
  [() => workflow.cue, () => workflow.loading, nextAction],
  async ([cue, loading]) => {
    if (
      !cue ||
      loading ||
      !workflow.progress ||
      workflow.progress.dismissed ||
      workflow.progress.collapsed
    ) {
      return;
    }
    await nextTick();
    hintUnavailable.value = false;
    if (cue.kind === 'client-name') {
      spotlight.value = {
        targets: ['collect-client-name'],
        title: 'Enter the client’s name',
        body: 'Use the name you want to see in your client directory.',
      };
    } else if (cue.kind === 'link-access') {
      spotlight.value = {
        targets: ['collect-link-access'],
        title: 'Choose Write access',
        body: 'Write allows uploads only. Check the expiry date, then select Create Link.',
      };
    } else {
      spotlight.value = nextAction.value;
    }
  },
  { flush: 'post' },
);
watch(
  () => workflow.step,
  (step, previous) => {
    if (step === previous) {
      return;
    }
    tucked.value = false;
    dockAtTop.value = false;
    showAllSteps.value = false;
    clearHint();
    if (workflow.guidance && step !== 4) {
      workflow.guide(step === 2 ? 'copy' : 'start');
    }
  },
);
watch(
  () => [workflow.progress?.dismissed, workflow.progress?.collapsed],
  () => {
    if (workflow.progress?.dismissed || workflow.progress?.collapsed) {
      clearHint();
      workflow.stopGuidance();
    }
  },
);
async function refresh() {
  await auth.fetchMe(true);
  if (workflow.allowed) {
    await workflow.reconcile();
  }
}
watch(
  () => route.path,
  async () => {
    clearHint();
    await workflow.reconcile();
    if (workflow.guidance) {
      workflow.guide('start');
    }
  },
);
watch(
  () => workflow.link?.expires_at,
  (expires) => {
    clearTimeout(expiryTimer);
    if (expires && Date.parse(expires) > Date.now()) {
      expiryTimer = setTimeout(
        () => void workflow.reconcile(),
        Math.min(Date.parse(expires) - Date.now() + 50, 2147483647),
      );
    }
  },
);
const stopClients = clients.$onAction(({ name, after }) => {
  after((result) => {
    if (!workflow.progress) {
      return;
    }
    if (name === 'createClient' && !workflow.progress.clientId) {
      workflow.selectClient(result.id);
    }
    if (name === 'deleteClient') {
      void workflow.reconcile();
    }
  });
});
const stopLinks = useSecureLinksStore().$onAction(({ name, args, after }) => {
  after((result) => {
    if (!workflow.progress) {
      return;
    }
    if (name === 'createLink' && args[0] === workflow.progress.clientId) {
      if (['write', 'readwrite'].includes(result.access_type)) {
        workflow.selectLink(result.id);
        if (result.email_queued) {
          workflow.shared(args[0], result.id, 'email_queued');
        }
      }
      void workflow.reconcile();
    }
    if (name === 'revokeLink') {
      void workflow.reconcile();
    }
  });
});
onMounted(() => {
  window.addEventListener('focus', refresh);
  void axios
    .get('/api/help')
    .then(({ data }) => {
      workflow.guideBase = data.guide_url || '';
    })
    .catch(() => undefined);
});
onBeforeUnmount(() => {
  stopClients();
  stopLinks();
  clearHint();
  clearTimeout(expiryTimer);
  window.removeEventListener('focus', refresh);
});
</script>

<template>
  <Teleport to="body">
    <aside
      v-if="workflow.allowed && workflow.progress && !workflow.progress.dismissed"
      class="collection-dock collection-theme"
      :style="dockAtTop ? { top: '12px', bottom: 'auto' } : undefined"
      role="region"
      aria-label="Collect a document"
    >
      <Panel
        header="Collect a document"
        toggleable
        :collapsed="panelCollapsed"
        :toggleButtonProps="{
          'aria-label': panelCollapsed ? 'Expand checklist' : 'Minimize checklist',
          title: panelCollapsed ? 'Expand checklist' : 'Minimize checklist',
          severity: 'secondary',
          text: true,
          rounded: true,
        }"
        :pt="{
          contentContainer: { role: 'group', 'aria-labelledby': 'collection-title' },
          transition: { css: false },
        }"
        @update:collapsed="minimize"
      >
        <template #toggleicon="{ collapsed }">
          <i :class="['oi', collapsed ? 'oi-caret-up' : 'oi-caret-down']" aria-hidden="true" />
        </template>
        <template #header>
          <div class="collection-heading">
            <span class="collection-document-mark" aria-hidden="true"
              ><i class="pi pi-file"
            /></span>
            <div>
              <span class="collection-eyebrow"
                >GETTING STARTED <span class="collection-count">{{ workflow.step }}/4</span></span
              >
              <h2 id="collection-title">Collect a document</h2>
            </div>
          </div>
        </template>
        <template #icons
          ><Button
            icon="pi pi-times"
            severity="secondary"
            text
            rounded
            aria-label="Dismiss checklist"
            title="Dismiss checklist (resume from Help)"
            @click="dismiss"
        /></template>
        <div class="collection-body">
          <div v-if="workflow.client" class="collection-client-context">
            <i class="pi pi-folder" aria-hidden="true" /><span>{{ workflow.client.name }}</span>
          </div>
          <Message v-if="workflow.error" severity="error" :closable="false">{{
            workflow.error
          }}</Message>
          <span v-if="workflow.loading" class="collection-refresh" role="status"
            >Checking progress…</span
          >
          <Stepper
            :value="current + 1"
            linear
            class="collection-steps"
            :class="{ 'collection-steps-overview': showAllSteps }"
            aria-label="Document collection steps"
          >
            <StepItem
              v-for="(title, index) in titles"
              :key="title"
              :value="index + 1"
              :class="{ 'collection-inactive-step': current !== index }"
            >
              <Step v-slot="{ a11yAttrs }" asChild>
                <div
                  v-bind="a11yAttrs.root"
                  :class="[
                    'collection-step',
                    { 'is-done': workflow.step > index, 'is-current': current === index },
                  ]"
                >
                  <button
                    v-bind="a11yAttrs.header"
                    type="button"
                    :aria-selected="current === index"
                    :tabindex="current === index ? 0 : -1"
                    :disabled="current !== index || workflow.loading"
                    :aria-description="
                      current === index ? 'Select to highlight this step in the app.' : undefined
                    "
                    @click="highlightStep(index)"
                  >
                    <span class="collection-step-number"
                      ><i
                        v-if="workflow.step > index"
                        class="pi pi-check"
                        aria-label="Completed"
                      /><span v-else>{{ index + 1 }}</span></span
                    >
                    <span class="collection-step-title">{{ title }}</span>
                    <span v-if="index === 3 && workflow.step === 3" class="collection-waiting"
                      >Waiting</span
                    >
                  </button>
                </div>
              </Step>
              <StepPanel :pt="{ transition: { css: false } }">
                <div
                  v-if="current === index"
                  class="collection-action"
                  :aria-busy="workflow.loading"
                >
                  <template v-if="index === 0">
                    <p>
                      In Clients, select a client’s name to open their workspace. To add one, select
                      New Client.
                    </p>
                  </template>
                  <template v-else-if="index === 1">
                    <Message v-if="workflow.progress.linkId" severity="warn" :closable="false"
                      >This link no longer accepts uploads. Create a new link or copy an active
                      upload link.</Message
                    >
                    <p>In the client workspace, open Secure Links and select New Link.</p>
                    <div class="collection-permission">
                      <i class="pi pi-lock" aria-hidden="true" />
                      <div>
                        <strong>Write access · recommended</strong
                        ><span
                          >Recipients can upload to this workspace but cannot view or download its
                          files.</span
                        >
                      </div>
                    </div>
                    <p>You can also copy an existing active upload link from Secure Links.</p>
                  </template>
                  <template v-else-if="index === 2">
                    <p>
                      In Secure Links, select Copy beside your upload link. Paste it into your
                      message to the recipient.
                    </p>
                    <Message
                      v-if="workflow.link?.access_type === 'readwrite'"
                      severity="warn"
                      :closable="false"
                      >Read & Write lets recipients view and download files in this
                      workspace.</Message
                    >
                    <span class="collection-fine-print">Copying a link does not send it.</span>
                  </template>
                  <template v-else>
                    <Message
                      :severity="workflow.step === 4 ? 'success' : 'info'"
                      :closable="false"
                      >{{
                        workflow.step === 4
                          ? 'A file was uploaded through this link.'
                          : 'Your link is ready to share.'
                      }}</Message
                    >
                    <p v-if="workflow.step < 4 && workflow.progress.shared === 'copied'">
                      Link copied. Paste it into your message to the recipient.
                    </p>
                    <p v-if="workflow.step < 4 && workflow.progress.shared === 'email_queued'">
                      Email queued. Delivery is not confirmed.
                    </p>
                    <p>
                      To check for uploads, open Clients and select the client’s name. Reload their
                      workspace to see new files.
                    </p>
                  </template>
                </div>
              </StepPanel>
            </StepItem>
          </Stepper>
          <Message v-if="hintUnavailable" severity="secondary" :closable="false"
            >This control isn’t visible here. Follow the step’s instructions to find it.</Message
          >
        </div>
        <template #footer
          ><div class="collection-footer">
            <a :href="guide" target="_blank" rel="noopener noreferrer"
              ><i class="pi pi-book" aria-hidden="true" />Read the guide<i
                class="pi pi-arrow-up-right"
                aria-hidden="true" /></a
            ><Button
              v-if="mobile"
              :label="showAllSteps ? 'Current step' : 'All steps'"
              :aria-pressed="showAllSteps"
              severity="secondary"
              text
              class="collection-overview-toggle"
              @click="showAllSteps = !showAllSteps"
            /><Button
              icon="pi pi-replay"
              severity="secondary"
              text
              rounded
              aria-label="Restart checklist"
              title="Restart checklist (resets tour progress only)"
              @click="
                clearHint();
                workflow.restart();
              "
            /></div
        ></template>
      </Panel>
    </aside>
  </Teleport>
  <CollectionSpotlight
    :request="spotlight"
    @close="clearHint"
    @unavailable="unavailable"
    @obstructed="
      (position) => {
        tucked = true;
        dockAtTop = position === 'top';
      }
    "
  />
</template>
