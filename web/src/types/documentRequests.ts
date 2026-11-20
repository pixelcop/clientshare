export type DocumentRequestStatus = 'needed' | 'submitted' | 'replacement' | 'accepted';

export const documentRequestLabels: Record<DocumentRequestStatus, string> = {
  needed: 'Not submitted',
  submitted: 'Submitted for review',
  replacement: 'Replacement needed',
  accepted: 'Accepted',
};

export interface DocumentRequestSummary {
  id: string;
  title: string;
  expires_at: string;
  revoked?: boolean;
}

export interface DocumentRequestItem {
  id: string;
  name: string;
  instructions: string;
  status: DocumentRequestStatus;
  reason: string;
  round: number;
  files: { id?: string; name: string; round: number }[];
  history?: {
    round: number;
    status: 'replacement' | 'accepted';
    reason: string;
    reviewed_at: string;
  }[];
}

export interface DocumentRequest extends DocumentRequestSummary {
  items: DocumentRequestItem[];
  link?: string;
  folder_id?: string;
  folder_name?: string;
  email_status?: 'queued' | 'unavailable' | 'no_recipients';
  email_count?: number;
}

export type RecipientDocumentRequest = Pick<DocumentRequest, 'title' | 'expires_at' | 'items'>;

export function requestFileRoute(clientId: string, requestId: string, fileId: string) {
  return {
    name: 'FileView',
    params: { fileId },
    query: { requestClient: clientId, request: requestId },
  };
}
