import type { LocationQuery, LocationQueryRaw } from 'vue-router';

import type { FileListingOptions, FileSortField } from '@/stores/fileManager';

export const filePageSizes = [10, 20, 50];

export function parseFileListingQuery(query: LocationQuery): FileListingOptions {
  const page = typeof query.page === 'string' ? Number(query.page) : 1;
  const pageSize = typeof query.page_size === 'string' ? Number(query.page_size) : 20;
  const sortField =
    typeof query.sort_by === 'string' && ['filename', 'size', 'uploaded_at'].includes(query.sort_by)
      ? (query.sort_by as FileSortField)
      : 'uploaded_at';
  return {
    page: Number.isSafeInteger(page) && page > 0 ? page : 1,
    pageSize: filePageSizes.includes(pageSize) ? pageSize : 20,
    sortField,
    sortDirection: query.sort_dir === 'asc' ? 'asc' : 'desc',
  };
}

export function fileListingQuery(
  options: FileListingOptions,
  query: LocationQuery = {},
): LocationQueryRaw {
  const next: LocationQueryRaw = { ...query };
  delete next.page;
  delete next.page_size;
  delete next.sort_by;
  delete next.sort_dir;
  if (options.page !== 1) {
    next.page = String(options.page);
  }
  if (options.pageSize !== 20) {
    next.page_size = String(options.pageSize);
  }
  if (options.sortField !== 'uploaded_at') {
    next.sort_by = options.sortField;
  }
  if (options.sortDirection !== 'desc') {
    next.sort_dir = options.sortDirection;
  }
  return next;
}
