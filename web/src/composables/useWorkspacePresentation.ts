import { useAuthStore } from '@/stores/auth';
import { useFileManagerStore } from '@/stores/fileManager';

/** Last settled view, for display only. Never use this snapshot to authorize actions. */
export function useWorkspacePresentation() {
  const files = useFileManagerStore();
  const auth = useAuthStore();
  const read = () => ({
    context: files.context,
    breadcrumbs: files.breadcrumbs,
    items: files.items,
    files: files.files,
    folders: files.folders,
    pagination: files.pagination,
    canRead: files.canRead,
    canWrite: files.canWrite,
    loaded: files.loaded,
    isEmpty: files.isEmpty,
  });
  const presentation = shallowRef(read());

  watchEffect(() => {
    // Folder changes clear the live authorization snapshot synchronously. Keep
    // the old presentation until all of the destination's data arrives together.
    if (!files.loading) {
      presentation.value = read();
    }
  });
  watch(
    [
      () => auth.user?.tenant_id,
      () => auth.user?.id,
      () => auth.user?.role,
      () => auth.user?.session_id,
      () => auth.user?.link_id,
    ],
    () => {
      // An identity change is not navigation: discard the old view immediately.
      presentation.value = read();
    },
    { flush: 'sync' },
  );

  return computed(() => presentation.value);
}
