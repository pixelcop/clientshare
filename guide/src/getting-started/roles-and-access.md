# Roles and access explained

**Who:** Anyone choosing how to share access or trying to understand the actions they see.

## The four kinds of access

- **Admin:** manages the portal, its clients and users, secure links, and settings.
- **Manager:** works with clients, users, and documents. Secure-link management and portal settings are administrator actions.
- **Client:** signs in and works with the client workspaces assigned to that account. The user-creation form calls this role **Customer**; the directory may display **client**.
- **Secure-link recipient:** uses the permissions granted by a link, without needing a permanent account.

## Permissions at a glance

This table describes the standard app interface.

| Task | Admin | Manager | Client user | Link recipient |
| --- | --- | --- | --- | --- |
| Create a client | Yes | Yes | No | No |
| Edit/delete a client from the directory | Yes | Not shown | No | No |
| Manage users | Yes | Yes | No | No |
| Read/download client files | All clients | All clients | Assigned clients | Read or Read & Write |
| Upload files | All clients | All clients | Assigned clients | Write or Read & Write |
| Delete files/folders | Yes | Yes | No | No |
| Manage secure links | Yes | No | No | No |
| Change portal settings | Yes | No | No | No |
| Manage own passkeys | Yes | Yes | Yes | Requires an account |

Managers have broad portal access. Use the client/Customer role for an external recipient who should only see selected client workspaces.

## Account or link?

**Invite a user** when someone needs regular access using their own sign-in. Assign a client user only to the relevant clients. See [Invite and manage users](../clients/manage-users.md).

**Send a secure link** for a time-limited exchange. Choose Read to deliver documents, Write to collect uploads, or Read & Write for both. See [Permissions and expiry](../sharing/permissions-and-expiry.md).

::: important Link scope
Secure links grant access to a client workspace, not an individual file. Write links disable downloads, but should not be treated as a guarantee that filenames or folder structure are hidden.
:::

## Change or remove access

Edit a user's client assignments to change their workspace access. Revoke a secure link to stop that link being used. These are separate access paths: revoking a link does not remove a user's assigned access, and deleting a user does not automatically replace a review of outstanding links.
