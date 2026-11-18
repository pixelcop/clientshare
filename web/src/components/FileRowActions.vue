<script setup lang="ts">
import { Button, Menu } from 'openvue';
import type { MenuItem } from 'openvue/menuitem';

import { useAuthStore } from '@/stores/auth';
import { useFileManagerStore } from '@/stores/fileManager';
import type { FileItem } from '@/types/models';
import type { SharingScope } from '@/types/sharing';

import ConfirmDeleteModal from './ConfirmDeleteModal.vue';
import SharingDialog from './SharingDialog.vue';

const { item } = defineProps<{ item: FileItem }>();
const emit = defineEmits<{ open: [item: FileItem] }>();
const fileManager = useFileManagerStore();
const auth = useAuthStore();
const menu = ref<InstanceType<typeof Menu> | null>(null);
const menuOpen = ref(false);
const showDelete = ref(false);
const showShare = ref(false);
const menuId = `item-actions-${item.id}`;
const isFolder = computed(() => item.type === 'folder');
const canDelete = computed(() => !!item.capabilities?.can_delete);
const canRead = computed(() => !!item.capabilities?.can_read);
const canShare = computed(
  () => !!auth.user && auth.user.role !== 'link' && !!item.capabilities?.can_share,
);
const sharingScope = computed<SharingScope>(() => ({
  kind: 'item',
  id: item.id,
  name: item.filename,
  itemType: item.type,
}));
const primaryLabel = 'Open';

function download() {
  if (isFolder.value) {
    void fileManager.downloadZip(item);
  } else {
    void fileManager.downloadFile(item);
  }
}
function open() {
  emit('open', item);
}
const actions = computed<MenuItem[]>(() => {
  const result: MenuItem[] = [];
  if (canRead.value) {
    result.push({
      label: primaryLabel,
      icon: isFolder.value ? 'oi oi-folder-open' : 'oi oi-eye',
      command: open,
    });
    result.push({
      label: isFolder.value ? 'Download ZIP' : 'Download',
      icon: 'oi oi-download',
      command: download,
    });
  }
  if (canShare.value) {
    result.push({
      label: 'Share',
      icon: 'oi oi-users',
      command: () => {
        showShare.value = true;
      },
    });
  }
  if (canDelete.value) {
    result.push(
      { separator: true },
      {
        label: 'Delete',
        icon: 'oi oi-trash',
        command: () => {
          showDelete.value = true;
        },
      },
    );
  }
  return result;
});
watch(
  () => fileManager.loading,
  (loading) => {
    if (loading) {
      menu.value?.hide();
      showDelete.value = false;
      showShare.value = false;
    }
  },
);
</script>

<template>
  <div class="row-actions" :data-menu-open="menuOpen" @click.stop>
    <div class="row-quick-actions">
      <Button
        v-if="canRead"
        icon="oi oi-download"
        :aria-label="`Download ${item.filename}${isFolder ? ' as ZIP' : ''}`"
        :title="isFolder ? 'Download ZIP' : 'Download'"
        severity="secondary"
        variant="text"
        @click.stop="download"
      />
    </div>
    <Button
      v-if="canShare"
      icon="oi oi-users"
      :aria-label="`Share ${item.filename}`"
      title="Share"
      aria-haspopup="dialog"
      :aria-expanded="showShare"
      :aria-controls="`sharing-item-${item.id}`"
      severity="secondary"
      variant="text"
      class="inline-share"
      @click.stop="showShare = true"
    />
    <Button
      v-if="actions.length"
      icon="oi oi-ellipsis-h"
      :aria-label="`More actions for ${item.filename}`"
      aria-haspopup="menu"
      :aria-controls="menuId"
      severity="secondary"
      variant="text"
      @click.stop="menu?.toggle($event)"
    />
    <Menu
      ref="menu"
      :id="menuId"
      :model="actions"
      popup
      :pt="{ itemLink: { class: 'min-h-11' } }"
      :aria-label="`Actions for ${item.filename}`"
      @show="menuOpen = true"
      @hide="menuOpen = false"
    />
    <ConfirmDeleteModal
      v-if="showDelete"
      :showDelete="showDelete"
      :item="item"
      @close="showDelete = false"
    />
    <SharingDialog
      v-if="showShare"
      :visible="showShare"
      :scope="sharingScope"
      @close="showShare = false"
    />
  </div>
</template>

<style scoped>
.row-actions {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  width: 132px;
}
.row-actions :deep(button) {
  width: 44px;
  height: 44px;
  flex: 0 0 44px;
}
.row-quick-actions {
  display: flex;
  visibility: hidden;
}
.inline-share {
  visibility: hidden;
}
.file-row:hover .row-quick-actions,
.file-row:focus-within .row-quick-actions,
.row-actions[data-menu-open='true'] .row-quick-actions {
  visibility: visible;
}
.file-row:hover .inline-share,
.file-row:focus-within .inline-share,
.row-actions[data-menu-open='true'] .inline-share {
  visibility: visible;
}
/* A hidden control must still become visible as keyboard focus enters its row.
   The filename is the first stop; then the quick actions are available. */
.row-actions :deep(button:focus-visible) {
  outline: 2px solid var(--p-primary-color);
  outline-offset: -2px;
}
@media (hover: none), (pointer: coarse) {
  .row-quick-actions {
    visibility: visible;
  }
  .inline-share {
    visibility: visible;
  }
}
@media (max-width: 480px) {
  .row-actions {
    width: 44px;
  }
  .row-quick-actions {
    display: none;
  }
  .inline-share {
    display: none;
  }
}
</style>
