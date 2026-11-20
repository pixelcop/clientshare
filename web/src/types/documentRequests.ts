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
  files: {
    id?: string;
    name: string;
    round: number;
    submitted_at?: string;
    comment_count?: number;
  }[];
  history?: {
    round: number;
    status: 'replacement' | 'accepted';
    reason: string;
    reviewed_at: string;
  }[];
}

export interface DocumentRequest extends DocumentRequestSummary {
  items: DocumentRequestItem[];
  description?: string;
  updates?: { id: string; description: string; created_at: string }[];
  link?: string;
  folder_id?: string;
  folder_name?: string;
  email_status?: 'queued' | 'unavailable' | 'no_recipients';
  email_count?: number;
}

export type RecipientDocumentRequest = Pick<
  DocumentRequest,
  'title' | 'expires_at' | 'items' | 'description' | 'updates'
>;

export function requestFileRoute(
  clientId: string,
  requestId: string,
  fileId: string,
  comments = false,
) {
  return {
    name: 'FileView',
    params: { fileId },
    query: { requestClient: clientId, request: requestId, ...(comments && { comments: '1' }) },
  };
}
