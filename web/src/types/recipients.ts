export interface RecipientContact {
  id: string;
  name: string;
  email: string;
  clients: { id: string; name: string }[];
  current_client: boolean;
  workspace_access: boolean;
}

export function recipientContext(contact: RecipientContact, clientId?: string): string {
  if (!clientId) {
    return contact.clients.map((client) => client.name).join(' · ') || 'No client association';
  }
  if (contact.workspace_access) {
    return 'Workspace access';
  }
  return contact.current_client ? 'Associated with this client' : 'Not in this client';
}
