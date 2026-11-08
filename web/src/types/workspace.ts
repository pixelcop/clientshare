import type { AccessCapabilities, FileItem, PaginatedResult } from './models';

export interface WorkspaceContext {
  folder: FileItem;
  client_id: string;
  client_name: string;
  root_folder_id: string;
  capabilities: AccessCapabilities;
  client_capabilities: AccessCapabilities;
  can_manage_client: boolean;
  can_manage_queue: boolean;
}

export interface WorkspaceListing extends PaginatedResult<FileItem> {
  context: WorkspaceContext;
}
