<script setup lang="ts">
import axios from 'axios';

import Breadcrumb from '@/components/Breadcrumb.vue';
import FileList from '@/components/FileList.vue';
import PublicLinkSignup from '@/components/PublicLinkSignup.vue';
import { useAuthStore } from '@/stores/auth';
import { useFileManagerStore } from '@/stores/fileManager';
import { usePublicLinkStore } from '@/stores/publicLink';

const { folderId } = defineProps<{ folderId?: string }>();
const router = useRouter();
const route = useRoute();

const publicLink = usePublicLinkStore();
const fileManager = useFileManagerStore();
const auth = useAuthStore();

const accessType = computed(() => publicLink.link?.access_type || auth.user?.access_type || 'read');

const accessLabel = computed(() => {
  if (['read', 'view'].includes(accessType.value)) {
    return 'Read-only';
  }
  if (accessType.value === 'write') {
    return 'Upload-only';
  }
  return 'Read & upload';
});

const accessDescription = computed(() => {
  if (['read', 'view'].includes(accessType.value)) {
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

async function openFolder(id?: string) {
  if (id && !folderId) {
    await router.replace({ name: 'FolderView', params: { folderId: id }, query: route.query });
    return;
  }
  await fileManager.changeFolder(id);
}

async function loadLink() {
  const token = localStorage.getItem('link');
  if (!token) {
    if (folderId || auth.user?.file_id) {
      await openFolder(folderId || auth.user?.file_id);
    } else {
      try {
        const response = await axios.get('/api/folders');
        await openFolder(response.data.context.folder.id);
      } catch {
        fileManager.error = 'This link is unavailable. Open the original sharing link again.';
      }
    }
    return;
  }
  await publicLink.validateLink(token);
  if (!publicLink.error) {
    if (publicLink.target?.type === 'file') {
      await router.replace(`/files/${publicLink.target.id}`);
      return;
    }
    await openFolder(
      folderId || publicLink.target?.id || publicLink.client?.root_folder_id || undefined,
    );
  }
}

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
        <Breadcrumb heading class="min-w-0 flex-1" />
        <div class="flex shrink-0 items-center gap-3">
          <span
            class="inline-flex items-center gap-2 rounded-full px-3 py-1 text-sm font-medium"
            :class="
              ['read', 'view'].includes(accessType)
                ? 'bg-blue-50 text-blue-700'
                : accessType === 'write'
                  ? 'bg-amber-50 text-amber-700'
                  : 'bg-emerald-50 text-emerald-700'
            "
          >
            <i class="oi oi-shield" />
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
        <FileList />
      </div>
    </section>

    <PublicLinkSignup />
  </div>
</template>
