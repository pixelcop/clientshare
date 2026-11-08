# Roles and access explained

**Who:** Anyone choosing how to share access or interpreting the actions they see.

## Accounts and scope

- **Admin:** has tenant-wide authority over clients, users, sharing and settings.
- **Manager:** works with assigned clients. An administrator can explicitly grant
  full access to all current and future clients. Managers present before the
  scoped-sharing upgrade retain full access; new managers default to assigned-only.
- **Client / Customer:** a named account with client memberships, direct item
  shares, or both. An item invitation does not assign the whole client.
- **Link recipient:** uses only that link's scope and permissions, without an
  account. A bearer link never grants sharing or staff-management authority.

## Permissions and inheritance

View/download, upload, deletion and resharing are separate permissions. Upload
does not mean editing or deletion. You can invite others or create links only
where you have sharing authority, and cannot give more access than you hold.

Client membership applies throughout that client. Folder shares include current
and future descendants. Direct shares add permissions: a narrower file share
cannot hide it from an existing client member or reduce inherited access.
Restricted folders that break inheritance are not supported.

| Task | Who can do it |
| --- | --- |
| Read/download | Named accounts with read access; read-capable link recipients within scope |
| Upload/create folders | Accounts or links with upload access to the destination |
| Delete | Named accounts with explicit delete authority; never bearer links |
| Share files/folders | Named accounts with resharing authority on that item |
| Invite to the whole client | Accounts with client-level sharing authority |
| Remove client members | Authorized staff, or a client inviter managing their own invitees |
| Change tenant identities, staff roles or full manager access | Administrators |
| Change portal settings | Administrators |

## Account or link?

Use **Invite by email** in an item's **Share** dialog for identity-bound access
to one file or folder. Accepted direct shares appear in **Shared with me**.
Use **Manage client access** to invite someone to the entire workspace. Client
users cannot take ownership of independently authorized members or manage staff.

A secure link is a separate bearer credential. Choose a client, folder or file
scope deliberately. File links are view/download only; folder and client links
can also collect uploads. **Upload only** is private collection with no browsing
of existing content. See [Share a file or folder](../sharing/share-items.md).

## Change or remove access

Removing a direct grant can leave inherited access. Removing client membership
also removes that person's direct grants and pending invitations within that
client. Their onward shares remain until independently revoked or expired.
Known bearer links still work unless explicitly revoked; the removal dialog
offers an authorized revoke-all-links option. Copies already downloaded remain.
