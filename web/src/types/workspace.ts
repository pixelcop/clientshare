import type { FileItem, PaginatedResult } from './models';

export interface WorkspaceContext {
  folder: FileItem;
  client_id: string;
  client_name: string;
  root_folder_id: string;
}

export interface WorkspaceListing extends PaginatedResult<FileItem> {
  context: WorkspaceContext;
}
