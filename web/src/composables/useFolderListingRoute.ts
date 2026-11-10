import { useFileManagerStore } from '@/stores/fileManager';
import { fileListingQuery, parseFileListingQuery } from '@/utils/fileListingQuery';

/** The route is the source of truth for listing controls, including Back/Forward. */
export function useFolderListingRoute() {
  const route = useRoute();
  const router = useRouter();
  const files = useFileManagerStore();

  watch(
    [() => route.params.folderId, () => route.query],
    () => {
      // Set options before the child view changes folders. A different folder
      // will fetch once through changeFolder, not once for each query field.
      files.configureListing(
        parseFileListingQuery(route.query),
        route.params.folderId === files.currentFolder,
      );
    },
    { immediate: true, flush: 'sync' },
  );

  watchEffect(() => {
    if (files.loading || !files.loaded) {
      return;
    }
    const query = fileListingQuery(
      {
        ...parseFileListingQuery(route.query),
        page: files.pagination.page,
        pageSize: files.pagination.page_size,
      },
      route.query,
    );
    if (route.params.folderId !== files.currentFolder) {
      return;
    }
    if (
      ['page', 'page_size', 'sort_by', 'sort_dir'].some((key) => route.query[key] !== query[key])
    ) {
      // Includes server-clamped bookmarks after deletions. Replacing keeps the
      // original navigation entry, rather than adding a phantom extra page.
      void router.replace({ query });
    }
  });
}
