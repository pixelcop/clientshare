// Directory traversal and uploads are deliberately sequential: parents must
// exist before children, and stopping must not start another request.
/* eslint-disable no-await-in-loop, no-restricted-syntax, no-continue */
import { computed, onBeforeUnmount, ref } from 'vue';

import { useFileManagerStore } from '@/stores/fileManager';
import type { AccessCapabilities } from '@/types/models';

export function useUploadController(options: {
  canNotify: () => boolean;
  notifyClient: () => boolean;
  onFiles?: (files: File[]) => void;
  onUploaded?: () => void;
}) {
  const fileManager = useFileManagerStore();

  const dragging = ref(false);
  const canceled = ref(false);
  const dropOverlayActive = ref(false);
  const fileInput = ref<HTMLInputElement | null>(null);
  const directoryInput = ref<HTMLInputElement | null>(null);
  let dragLeaveTimer: ReturnType<typeof setTimeout> | null = null;
  let finishTimer: ReturnType<typeof setTimeout> | null = null;
  let destinationFolderId: string | null = null;
  let destinationCapabilities: AccessCapabilities | undefined;
  let shouldNotify = false;
  let transfer: AbortController | null = null;
  const preparing = ref(false);
  const stopped = ref(false);
  const busy = computed(() => preparing.value || uploading.value || showRename.value);

  function captureDestination() {
    if (!fileManager.canWrite || busy.value || !fileManager.currentFolder) {
      return false;
    }
    destinationFolderId = fileManager.currentFolder;
    destinationCapabilities = { ...fileManager.capabilities };
    shouldNotify = options.canNotify() && options.notifyClient();
    transfer = new AbortController();
    stopped.value = false;
    preparing.value = true;
    return true;
  }

  function stopUpload() {
    stopped.value = true;
    transfer?.abort();
    uploadError.value = 'Upload stopped. Completed files and folders were kept.';
  }

  function checkStopped() {
    if (stopped.value) {
      throw new Error('Upload stopped. Completed files and folders were kept.');
    }
  }

  interface PendingRename {
    id: string;
    file: File;
    originalName: string;
    name: string;
    source: 'existing' | 'duplicate';
  }

  type FileSystemEntryLike = FileSystemEntry;
  type FileSystemFileEntryLike = FileSystemFileEntry;
  type FileSystemDirectoryReaderLike = FileSystemDirectoryReader;
  type FileSystemDirectoryEntryLike = FileSystemDirectoryEntry;
  type DirectorySelectableFile = File & {
    webkitRelativePath?: string;
  };
  type HTMLInputElementWithDirectoryEntries = HTMLInputElement & {
    webkitEntries?: FileSystemEntryLike[];
  };

  interface DroppedTreeFile {
    file: File;
    relativePath: string;
  }

  interface DroppedDirectoryTree {
    files: DroppedTreeFile[];
    folders: string[];
  }

  interface FolderUploadGroup {
    relativeFolderPath: string;
    files: File[];
  }

  const uploading = ref(false);
  const uploadProgress = ref(0);
  const uploadError = ref<string | null>(null);

  const showRename = ref(false);
  const renameItems = ref<PendingRename[]>([]);
  const renameSaving = ref(false);
  const renameError = ref<string | null>(null);
  const pendingFiles = ref<File[]>([]);

  const emit = (event: 'files' | 'uploaded', files?: File[]) => {
    if (event === 'files' && files) {
      options.onFiles?.(files);
    }
    if (event === 'uploaded') {
      options.onUploaded?.();
    }
  };

  function resetUploadFeedback() {
    uploadError.value = null;
  }

  function clearDragLeaveTimer() {
    if (dragLeaveTimer) {
      clearTimeout(dragLeaveTimer);
      dragLeaveTimer = null;
    }
  }

  function clearDropOverlay() {
    dragging.value = false;
    dropOverlayActive.value = false;
    clearDragLeaveTimer();
    window.removeEventListener('keydown', handleEsc);
  }

  function finishUpload() {
    if (finishTimer) {
      clearTimeout(finishTimer);
    }
    finishTimer = setTimeout(() => {
      uploading.value = false;
      uploadProgress.value = 0;
      clearDropOverlay();
      finishTimer = null;
    }, 2000);
  }

  function getUploadErrorMessage(error: unknown, fallback: string) {
    if (error instanceof Error && error.message) {
      return error.message;
    }

    if (typeof error === 'object' && error !== null) {
      const response = Reflect.get(error, 'response');
      if (typeof response === 'object' && response !== null) {
        const data = Reflect.get(response, 'data');
        if (typeof data === 'object' && data !== null) {
          const message = Reflect.get(data, 'error');
          if (typeof message === 'string' && message.length > 0) {
            return message;
          }
        }
      }
    }

    return fallback;
  }

  const JUNK_FILE_NAMES = new Set(['.ds_store', 'thumbs.db', 'desktop.ini', 'ehthumbs.db']);

  function isJunkFile(name: string) {
    const lower = name.toLowerCase();
    return JUNK_FILE_NAMES.has(lower) || lower.startsWith('._');
  }

  function splitRelativePath(path: string) {
    const segments = path.split('/').filter((segment) => segment.length > 0 && segment !== '.');
    if (segments.some((segment) => segment === '..')) {
      throw new Error('Dropped folder contains an invalid path.');
    }
    return segments;
  }

  function joinRelativePath(...parts: string[]) {
    return parts.flatMap((part) => splitRelativePath(part)).join('/');
  }

  function getParentRelativePath(path: string) {
    const segments = splitRelativePath(path);
    segments.pop();
    return segments.join('/');
  }

  function sortRelativePaths(paths: Iterable<string>) {
    return Array.from(paths).sort((left, right) => {
      const depthDiff = splitRelativePath(left).length - splitRelativePath(right).length;
      if (depthDiff !== 0) {
        return depthDiff;
      }
      return left.localeCompare(right);
    });
  }

  function readFileEntry(entry: FileSystemFileEntryLike) {
    return new Promise<File>((resolve, reject) => {
      entry.file(resolve, reject);
    });
  }

  function readDirectoryBatch(reader: FileSystemDirectoryReaderLike) {
    return new Promise<FileSystemEntryLike[]>((resolve, reject) => {
      reader.readEntries(resolve, reject);
    });
  }

  async function readAllDirectoryEntries(entry: FileSystemDirectoryEntryLike) {
    const reader = entry.createReader();
    const entries: FileSystemEntryLike[] = [];

    while (true) {
      const batch = await readDirectoryBatch(reader);
      if (batch.length === 0) {
        return entries;
      }
      entries.push(...batch);
    }
  }

  async function collectDroppedEntryTree(
    entry: FileSystemEntryLike,
    parentPath = '',
  ): Promise<DroppedDirectoryTree> {
    if (entry.isFile) {
      if (isJunkFile(entry.name)) {
        return { folders: [], files: [] };
      }
      const file = await readFileEntry(entry as FileSystemFileEntryLike);
      return {
        folders: [],
        files: [{ file, relativePath: joinRelativePath(parentPath, file.name) }],
      };
    }

    if (entry.isDirectory) {
      const directoryPath = joinRelativePath(parentPath, entry.name);
      const childEntries = await readAllDirectoryEntries(entry as FileSystemDirectoryEntryLike);
      const folders = [directoryPath];
      const files: DroppedTreeFile[] = [];

      for (const childEntry of childEntries) {
        const childTree = await collectDroppedEntryTree(childEntry, directoryPath);
        folders.push(...childTree.folders);
        files.push(...childTree.files);
      }

      return { folders, files };
    }

    return { folders: [], files: [] };
  }

  function validateDroppedDirectoryTree(tree: DroppedDirectoryTree) {
    const seenFiles = new Set<string>();

    for (const item of tree.files) {
      const key = item.relativePath.toLowerCase();
      if (seenFiles.has(key)) {
        throw new Error(`The dropped selection contains duplicate files at ${item.relativePath}.`);
      }
      seenFiles.add(key);
    }
  }

  async function collectDroppedDirectoryTree(dataTransfer: DataTransfer) {
    const items = Array.from(dataTransfer.items || []).filter((item) => item.kind === 'file');
    if (items.length === 0) {
      return null;
    }

    const rootEntries = items
      .map((item) => item?.webkitGetAsEntry())
      .filter((entry): entry is FileSystemEntryLike => !!entry);

    if (!rootEntries.some((entry) => entry.isDirectory)) {
      return null;
    }

    return collectDirectoryTreeFromEntries(rootEntries);
  }

  async function collectDirectoryTreeFromEntries(entries: FileSystemEntryLike[]) {
    if (!entries.some((entry) => entry.isDirectory)) {
      return null;
    }

    const folders = new Set<string>();
    const files: DroppedTreeFile[] = [];

    for (const entry of entries) {
      const tree = await collectDroppedEntryTree(entry);
      tree.folders.forEach((folder) => folders.add(folder));
      files.push(...tree.files);
    }

    const droppedTree = {
      folders: sortRelativePaths(folders),
      files,
    };

    validateDroppedDirectoryTree(droppedTree);
    return droppedTree;
  }

  function collectDirectoryTreeFromFiles(files: File[]) {
    const selectedFiles = files as DirectorySelectableFile[];
    const relativePaths = selectedFiles
      .map((file) => file.webkitRelativePath?.trim() || '')
      .filter((path) => path.length > 0);

    if (relativePaths.length === 0) {
      return null;
    }

    const folders = new Set<string>();
    const treeFiles: DroppedTreeFile[] = [];

    selectedFiles.forEach((file) => {
      const relativePath = file.webkitRelativePath?.trim();
      if (!relativePath) {
        return;
      }

      if (isJunkFile(file.name)) {
        return;
      }

      const normalizedPath = joinRelativePath(relativePath);
      const segments = splitRelativePath(normalizedPath);

      for (let depth = 1; depth < segments.length; depth += 1) {
        folders.add(segments.slice(0, depth).join('/'));
      }

      treeFiles.push({
        file,
        relativePath: normalizedPath,
      });
    });

    const selectedTree = {
      folders: sortRelativePaths(folders),
      files: treeFiles,
    };

    validateDroppedDirectoryTree(selectedTree);
    return selectedTree;
  }

  async function collectDirectoryInputTree(input: HTMLInputElement) {
    const entryInput = input as HTMLInputElementWithDirectoryEntries;
    const entries = Array.from(entryInput.webkitEntries || []).filter(
      (entry): entry is FileSystemEntryLike => !!entry,
    );

    if (entries.length > 0) {
      const treeFromEntries = await collectDirectoryTreeFromEntries(entries);
      if (treeFromEntries) {
        return treeFromEntries;
      }
    }

    return collectDirectoryTreeFromFiles(Array.from(input.files || []));
  }

  function buildFolderUploadGroups(files: DroppedTreeFile[]) {
    const groupedFiles = new Map<string, File[]>();

    files.forEach((item) => {
      const relativeFolderPath = getParentRelativePath(item.relativePath);
      const existingFiles = groupedFiles.get(relativeFolderPath) || [];
      existingFiles.push(item.file);
      groupedFiles.set(relativeFolderPath, existingFiles);
    });

    return sortRelativePaths(groupedFiles.keys()).map((relativeFolderPath) => ({
      relativeFolderPath,
      files: groupedFiles.get(relativeFolderPath) || [],
    }));
  }

  async function ensureFolderPath(
    relativeFolderPath: string,
    folderCache: Map<string, string>,
    rootFolderId: string,
  ) {
    if (relativeFolderPath.length === 0) {
      return rootFolderId;
    }

    let parentFolderId = rootFolderId;
    let currentPath = '';

    for (const segment of splitRelativePath(relativeFolderPath)) {
      checkStopped();
      currentPath = currentPath ? `${currentPath}/${segment}` : segment;
      const cachedFolderId = folderCache.get(currentPath);
      if (cachedFolderId) {
        parentFolderId = cachedFolderId;
        continue;
      }

      const folder = await fileManager.createFolder(segment, {
        destinationCapabilities,
        targetFolderId: parentFolderId,
        reuseExisting: true,
        signal: transfer?.signal,
      });
      if (!folder?.id) {
        throw new Error(`Failed to create folder ${currentPath}.`);
      }

      folderCache.set(currentPath, folder.id);
      parentFolderId = folder.id;
    }

    return parentFolderId;
  }

  async function validateFolderUploadGroups(
    groups: FolderUploadGroup[],
    folderCache: Map<string, string>,
    rootFolderId: string,
  ) {
    for (const group of groups) {
      const seenNames = new Set<string>();
      for (const file of group.files) {
        const key = file.name.toLowerCase();
        if (seenNames.has(key)) {
          const scope = group.relativeFolderPath || 'the current folder';
          throw new Error(
            `The dropped selection contains duplicate files in ${scope}: ${file.name}.`,
          );
        }
        seenNames.add(key);
      }

      const targetFolderId = await ensureFolderPath(
        group.relativeFolderPath,
        folderCache,
        rootFolderId,
      );
      const existingNames = await fileManager.checkExistingFilenames(
        group.files.map((file) => file.name),
        targetFolderId,
      );

      if (existingNames.length > 0) {
        const conflictingPath = joinRelativePath(group.relativeFolderPath, existingNames[0]);
        throw new Error(
          `The file ${conflictingPath} already exists. Rename it before uploading the folder.`,
        );
      }
    }
  }

  async function uploadFolderTree(tree: DroppedDirectoryTree) {
    const rootFolderId = destinationFolderId;
    if (!rootFolderId) {
      uploadError.value = 'No folder selected.';
      return;
    }

    uploading.value = true;
    uploadProgress.value = 0;
    resetUploadFeedback();

    try {
      const folderCache = new Map<string, string>([['', rootFolderId]]);
      for (const folderPath of tree.folders) {
        checkStopped();
        await ensureFolderPath(folderPath, folderCache, rootFolderId);
      }

      const groups = buildFolderUploadGroups(tree.files);
      await validateFolderUploadGroups(groups, folderCache, rootFolderId);

      const totalFiles = tree.files.length;
      let completedFiles = 0;

      for (const group of groups) {
        checkStopped();
        const progressBeforeGroup = completedFiles;
        const targetFolderId = await ensureFolderPath(
          group.relativeFolderPath,
          folderCache,
          rootFolderId,
        );
        const result = await fileManager.uploadFiles(
          group.files,
          shouldNotify,
          (percent) => {
            if (totalFiles === 0) {
              uploadProgress.value = percent;
              return;
            }

            const groupProgress = (percent / 100) * group.files.length;
            uploadProgress.value = Math.round(
              ((progressBeforeGroup + groupProgress) * 100) / totalFiles,
            );
          },
          {
            targetFolderId,
            destinationCapabilities,
            refreshCurrentFolder: false,
            signal: transfer?.signal,
          },
        );

        if (!result.ok) {
          const failedName = result.name || group.files[0]?.name || 'the file';
          const failedPath = joinRelativePath(group.relativeFolderPath, failedName);
          if (result.code === 'FILE_EXISTS') {
            throw new Error(
              `The file ${failedPath} already exists. Rename it before uploading the folder.`,
            );
          }
          throw new Error(result.error || `Failed to upload ${failedPath}.`);
        }

        completedFiles += group.files.length;
        if (totalFiles > 0) {
          uploadProgress.value = Math.round((completedFiles * 100) / totalFiles);
        }
      }

      if (tree.files.length === 0) {
        uploadProgress.value = 100;
      }
    } catch (error) {
      uploadError.value = getUploadErrorMessage(error, 'Failed to upload the dropped folder.');
    } finally {
      await fileManager.fetchFiles(fileManager.pagination.page, fileManager.pagination.page_size);
      emit('uploaded'); // Refresh pending notifications even after a partial transfer.
      finishUpload();
    }
  }

  function buildPendingRename(files: File[], conflicts: Set<string>) {
    const seen = new Set<string>();
    const items: PendingRename[] = [];
    files.forEach((file, index) => {
      const nameKey = file.name.toLowerCase();
      const isDuplicate = seen.has(nameKey);
      if (isDuplicate || conflicts.has(nameKey)) {
        items.push({
          id: `${Date.now()}-${index}-${file.name}`,
          file,
          originalName: file.name,
          name: file.name,
          source: isDuplicate ? 'duplicate' : 'existing',
        });
      }
      seen.add(nameKey);
    });
    return items;
  }

  async function handleUpload(files: File[]) {
    uploading.value = true;
    uploadProgress.value = 0;
    resetUploadFeedback();
    try {
      const result = await fileManager.uploadFiles(
        files,
        shouldNotify,
        (percent) => {
          uploadProgress.value = percent;
        },
        {
          targetFolderId: destinationFolderId,
          destinationCapabilities,
          refreshCurrentFolder: false,
          signal: transfer?.signal,
        },
      );
      if (!result.ok) {
        if (result.code === 'FILE_EXISTS' && result.name) {
          const existing = new Set([result.name.toLowerCase()]);
          const pending = buildPendingRename(files, existing);
          if (pending.length > 0) {
            renameItems.value = pending;
            renameError.value = null;
            pendingFiles.value = files;
            showRename.value = true;
            return;
          }
        }
        uploadError.value = result.error;
      }
    } catch (error) {
      uploadError.value = getUploadErrorMessage(error, 'Failed to upload files.');
    } finally {
      await fileManager.fetchFiles(fileManager.pagination.page, fileManager.pagination.page_size);
      emit('uploaded');
      finishUpload();
    }
  }

  function resetRenameState() {
    showRename.value = false;
    renameItems.value = [];
    renameSaving.value = false;
    renameError.value = null;
    pendingFiles.value = [];
  }

  function buildNameSet(files: File[], pending: PendingRename[]) {
    const pendingKeys = new Set(pending.map((item) => item.file));
    const names = new Set<string>();
    files.forEach((file) => {
      if (!pendingKeys.has(file)) {
        names.add(file.name.toLowerCase());
      }
    });
    pending.forEach((item) => {
      names.add(item.name.trim().toLowerCase());
    });
    return names;
  }

  function validateRename(files: File[], pending: PendingRename[], existingNames: Set<string>) {
    if (pending.length === 0) {
      return 'No files need renaming.';
    }
    const names = buildNameSet(files, pending);
    const seen = new Set<string>();
    for (const item of pending) {
      const trimmed = item.name.trim();
      if (trimmed.length === 0) {
        return 'Please enter a name for each file.';
      }
      const key = trimmed.toLowerCase();
      if (seen.has(key)) {
        return 'Each renamed file must have a unique name.';
      }
      if (existingNames.has(key)) {
        return 'Rename to a name that does not already exist in this folder.';
      }
      seen.add(key);
    }
    if (names.size !== files.length) {
      return 'Rename entries conflict with other selected files.';
    }
    return null;
  }

  async function resolveAndUpload(files: File[]) {
    resetUploadFeedback();
    const names = files.map((file) => file.name);
    if (!files.length) {
      return;
    }
    const existing = await fileManager.checkExistingFilenames(names, destinationFolderId);
    checkStopped();
    const existingSet = new Set(existing.map((name) => name.toLowerCase()));
    const pending = buildPendingRename(files, existingSet);
    if (pending.length > 0) {
      renameItems.value = pending;
      renameError.value = null;
      pendingFiles.value = files;
      showRename.value = true;
      return;
    }
    await handleUpload(files);
  }

  async function submitRename() {
    if (renameSaving.value) {
      return;
    }
    renameSaving.value = true;
    renameError.value = null;
    resetUploadFeedback();
    try {
      const names = renameItems.value.map((item) => item.name.trim());
      const existing = await fileManager.checkExistingFilenames(names, destinationFolderId);
      checkStopped();
      const existingSet = new Set(existing.map((name) => name.toLowerCase()));
      const validation = validateRename(pendingFiles.value, renameItems.value, existingSet);
      if (validation) {
        renameError.value = validation;
        return;
      }
      const renameMap = new Map<File, string>();
      renameItems.value.forEach((item) => renameMap.set(item.file, item.name.trim()));
      const files = pendingFiles.value;
      const targetNames = files.map((file) => renameMap.get(file) ?? file.name);
      uploading.value = true;
      if (finishTimer) {
        clearTimeout(finishTimer);
      }
      uploadProgress.value = 0;
      const result = await fileManager.uploadFiles(
        files,
        shouldNotify,
        (percent) => {
          uploadProgress.value = percent;
        },
        {
          targetFolderId: destinationFolderId,
          destinationCapabilities,
          refreshCurrentFolder: false,
          signal: transfer?.signal,
          targetNames,
        },
      );
      if (!result.ok) {
        renameError.value =
          result.code === 'FILE_EXISTS' && result.name
            ? `The file ${result.name} already exists. Choose a new name.`
            : result.error;
        return;
      }
      resetRenameState();
      await fileManager.fetchFiles(fileManager.pagination.page, fileManager.pagination.page_size);
      emit('uploaded');
      finishUpload();
    } catch (error: unknown) {
      renameError.value = getUploadErrorMessage(error, 'Failed to upload files.');
    } finally {
      renameSaving.value = false;
    }
  }

  function handleEsc(e: KeyboardEvent) {
    if (e.key === 'Escape' && dragging.value) {
      clearDropOverlay();
      canceled.value = true;
      window.removeEventListener('keydown', handleEsc);
    }
  }

  function onDragEnd(e: DragEvent) {
    if (!isFileDrag(e)) {
      return;
    }
    canceled.value = false;
    dragging.value = false;
    clearDragLeaveTimer();
    window.removeEventListener('keydown', handleEsc);
  }

  function onDragOver(e?: DragEvent) {
    if (!e || !isFileDrag(e) || !fileManager.canWrite || busy.value) {
      return;
    }
    if (e) {
      e.preventDefault();
    }
    if (dragLeaveTimer) {
      clearTimeout(dragLeaveTimer);
      dragLeaveTimer = null;
    }
    if (canceled.value) {
      return;
    }
    if (!dragging.value) {
      dragging.value = true;
      window.addEventListener('keydown', handleEsc);
    }
  }

  function onDragLeave() {
    if (dragLeaveTimer) {
      clearTimeout(dragLeaveTimer);
    }
    dragLeaveTimer = setTimeout(() => {
      dragging.value = false;
      canceled.value = false;
      window.removeEventListener('keydown', handleEsc);
      dragLeaveTimer = null;
    }, 80);
  }

  async function onDrop(e: DragEvent) {
    if (!isFileDrag(e) || !fileManager.canWrite || busy.value) {
      return;
    }
    e.preventDefault();
    if (canceled.value) {
      canceled.value = false;
      clearDropOverlay();
      return;
    }
    if (!e.dataTransfer) {
      clearDropOverlay();
      return;
    }
    if (!captureDestination()) {
      return;
    }

    dropOverlayActive.value = true;
    dragging.value = false;
    clearDragLeaveTimer();
    window.removeEventListener('keydown', handleEsc);

    try {
      const droppedTree = await collectDroppedDirectoryTree(e.dataTransfer);
      if (droppedTree) {
        emit(
          'files',
          droppedTree.files.map((item) => item.file),
        );
        await uploadFolderTree(droppedTree);
        return;
      }

      const files = Array.from(e.dataTransfer.files).filter((file) => !isJunkFile(file.name));
      emit('files', files);
      await resolveAndUpload(files);
    } catch (error) {
      uploadError.value = getUploadErrorMessage(error, 'Failed to upload files.');
      uploading.value = false;
      uploadProgress.value = 0;
      clearDropOverlay();
    } finally {
      preparing.value = false;
      if (!uploading.value) {
        clearDropOverlay();
      }
    }
  }

  function onClick() {
    if (!fileManager.canWrite || busy.value) {
      return;
    }
    fileInput.value?.click();
  }

  function chooseDirectory() {
    if (!fileManager.canWrite || busy.value) {
      return;
    }
    directoryInput.value?.click();
  }

  async function onFileChange(e: Event) {
    const input = e.target as HTMLInputElement;
    if (!input.files?.length || !captureDestination()) {
      return;
    }
    const files = Array.from(input.files).filter((file) => !isJunkFile(file.name));
    emit('files', files);
    try {
      await resolveAndUpload(files);
    } catch (error) {
      uploadError.value = getUploadErrorMessage(error, 'Failed to upload files.');
      uploading.value = false;
      uploadProgress.value = 0;
    }
    preparing.value = false;
    input.value = '';
  }

  async function onDirectoryChange(e: Event) {
    const input = e.target as HTMLInputElement;
    if (!input.files || input.files.length === 0 || !captureDestination()) {
      return;
    }

    const files = Array.from(input.files);
    emit('files', files);

    try {
      const tree = await collectDirectoryInputTree(input);
      if (tree) {
        await uploadFolderTree(tree);
      } else {
        await resolveAndUpload(files);
      }
    } catch (error) {
      uploadError.value = getUploadErrorMessage(error, 'Failed to upload the selected folder.');
      uploading.value = false;
      uploadProgress.value = 0;
    }

    preparing.value = false;
    input.value = '';
  }

  function isFileDrag(e: DragEvent) {
    return Array.from(e.dataTransfer?.types || []).includes('Files');
  }

  onBeforeUnmount(() => {
    window.removeEventListener('keydown', handleEsc);
    clearDragLeaveTimer();
    if (finishTimer) {
      clearTimeout(finishTimer);
    }
    transfer?.abort();
  });
  return {
    fileInput,
    directoryInput,
    dragging,
    busy,
    stopped,
    stopUpload,
    uploading,
    uploadProgress,
    uploadError,
    showRename,
    renameItems,
    renameSaving,
    renameError,
    resetRenameState,
    submitRename,
    onDragOver,
    onDragLeave,
    onDragEnd,
    onDrop,
    onClick,
    chooseDirectory,
    onFileChange,
    onDirectoryChange,
  };
}
