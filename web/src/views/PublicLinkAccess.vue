<script setup lang="ts">
import { storeToRefs } from 'pinia';

import Dropzone from '@/components/Dropzone.vue';
import FileList from '@/components/FileList.vue';
import PublicLinkSignup from '@/components/PublicLinkSignup.vue';
import { useFileManagerStore } from '@/stores/fileManager';
import { usePublicLinkStore } from '@/stores/publicLink';

const { folderId } = defineProps<{ folderId: string }>();

const publicLink = usePublicLinkStore();
const fileManager = useFileManagerStore();

const clientName = computed(() => publicLink.client?.name || 'Client Files');
const { accessType, canWrite } = storeToRefs(publicLink);

const accessLabel = computed(() => {
  if (accessType.value === 'read') {
    return 'Read-only';
  }
  if (accessType.value === 'write') {
    return 'Upload-only';
  }
  return 'Read & upload';
});

const accessDescription = computed(() => {
  if (accessType.value === 'read') {
    return 'You can view and download files.';
  }
  if (accessType.value === 'write') {
    return 'You can upload files, but downloads are disabled.';
  }
  return 'You can view, download, and upload files.';
});

const expiresAt = computed(() => {
  if (!publicLink.link?.expires_at) {
    return '';
  }
  return new Date(publicLink.link.expires_at).toLocaleString();
});

async function loadLink() {
  const token = localStorage.getItem('link');
  if (!token) {
    return;
  }
  await publicLink.validateLink(token);
  if (!publicLink.error) {
    await fileManager.changeFolder(folderId);
  }
}

function handleUploaded() {}

onMounted(() => {
  loadLink();
});

watch(
  () => folderId,
  (id) => {
    fileManager.changeFolder(id);
  },
);
</script>

<template>
  <div class="space-y-6">
    <section class="bg-white rounded-xl shadow-sm border border-slate-200 p-6">
      <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
        <div>
          <h1 class="text-2xl font-semibold text-slate-900">{{ clientName }}</h1>
          <p class="text-sm text-slate-500">Secure link access</p>
        </div>
        <div class="flex items-center gap-3">
          <span
            class="inline-flex items-center gap-2 rounded-full px-3 py-1 text-sm font-medium"
            :class="
              accessType === 'read'
                ? 'bg-blue-50 text-blue-700'
                : accessType === 'write'
                  ? 'bg-amber-50 text-amber-700'
                  : 'bg-emerald-50 text-emerald-700'
            "
          >
            <i class="pi pi-shield" />
            {{ accessLabel }}
          </span>
          <span v-if="expiresAt" class="text-xs text-slate-500">Expires {{ expiresAt }}</span>
        </div>
      </div>
      <p class="mt-3 text-slate-600">{{ accessDescription }}</p>
    </section>

    <section v-if="publicLink.error" class="bg-red-50 border border-red-200 rounded-xl p-4">
      <p class="text-sm text-red-700">{{ publicLink.error }}</p>
    </section>

    <section v-else class="space-y-4">
      <div v-if="publicLink.validating" class="text-sm text-slate-500">Validating link...</div>

      <div v-else>
        <div v-if="canWrite" class="space-y-3 mb-6">
          <Dropzone @uploaded="handleUploaded" notifyClient canNotify />
        </div>

        <FileList />
      </div>
    </section>

    <PublicLinkSignup />
  </div>
</template>
