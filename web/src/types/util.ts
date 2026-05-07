import { intlFormatDistance } from 'date-fns';

import type { FileItem, PaginationResponse } from './models';

export function isFolder(item: FileItem) {
  if (item.type) {
    return item.type === 'folder';
  }
  return !item.filename.includes('.') && item.size === 0;
}

export function getFileURL(item: FileItem, download?: boolean) {
  if (item.type === 'folder') {
    if (download) {
      // /api/folders/:id/download-zip
      return `/api/folders/${item.id}/download-zip`;
    }
    return `/api/folders/${item.id}`;
  }
  if (download) {
    return `/api/files/${item.id}/download`;
  }
  return `/api/files/${item.id}`; // TODO: no file endpoint yet
}

export function getFileType(item: FileItem) {
  const ext = item.filename.split('.').pop()?.toLowerCase() || '';
  if (ext === 'pdf') {
    return 'application/pdf';
  }
  if (['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg'].includes(ext)) {
    if (ext === 'svg') {
      return 'image/svg+xml';
    }
    return `image/${ext === 'jpg' ? 'jpeg' : ext}`;
  }
  if (['txt', 'json', 'xml', 'csv'].includes(ext)) {
    return `text/${ext === 'txt' ? 'plain' : ext}`;
  }
  if (['mp3', 'wav', 'ogg', 'flac', 'aac', 'm4a', 'opus'].includes(ext)) {
    return `audio/${ext === 'mp3' ? 'mpeg' : ext}`;
  }
  if (['doc', 'docx'].includes(ext)) {
    return 'application/msword';
  }
  if (['xls', 'xlsx'].includes(ext)) {
    return 'application/vnd.ms-excel';
  }
  return '';
}

export function formatSize(size: number) {
  if (!size) {
    return '-';
  }
  if (size < 1024) {
    return `${size} B`;
  }
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`;
  }
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

export function formatDate(dateStr: string) {
  if (!dateStr) {
    return '-';
  }
  return new Date(dateStr).toLocaleString();
}

export function formatDateNatural(dateStr: string) {
  if (!dateStr) {
    return '-';
  }
  const date = new Date(dateStr);
  // const i = interval(date, new Date());
  // const d = intervalToDuration(i);
  // if (!d.days || d.days < 21) {
  return intlFormatDistance(date, new Date(), { style: 'narrow' });
  // }
  // return formatDate(dateStr);
}

export function getFileIcon(item: FileItem) {
  if (isFolder(item)) {
    return 'la la-folder';
  }
  const fileType = getFileType(item);
  if (fileType.startsWith('image/')) {
    return 'la la-file-image';
  }
  if (fileType === 'application/pdf') {
    return 'la la-file-pdf';
  }
  if (fileType.startsWith('text/')) {
    return 'la la-file-alt';
  }
  if (fileType.startsWith('audio/')) {
    return 'la la-file-audio';
  }
  if (fileType.includes('msword') || fileType.includes('word')) {
    return 'la la-file-word';
  }
  if (fileType.includes('excel') || fileType.includes('spreadsheet')) {
    return 'la la-file-excel';
  }

  return 'la la-file';
}

export function defaultPagination(): PaginationResponse {
  return {
    total_items: 0,
    total_pages: 0,
    page: 1,
    page_size: 20,
    has_more: false,
    items_per_page: 0,
  };
}

export function joinPath(base: string, ...addition: string[]) {
  const parts = [base, ...addition]
    .filter(Boolean)
    .map((part) => part.replace(/^\/+|\/+$/g, ''))
    .filter(Boolean);
  if (parts.length === 0) {
    return '';
  }
  return parts.join('/');
}
