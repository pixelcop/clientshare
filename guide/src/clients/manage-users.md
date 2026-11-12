# Invite and manage users

**Who:** Administrators, and migrated Members with legacy organization management.

Create an individual user account for each person who needs ongoing access. A client workspace and a user account are separate records.

## Invite a user

1. Open **Users** and select **Add person**, or choose **Add User** from a client's action dropdown.
2. Enter the person's **Name** and **Email**.
3. Choose **Internal** or **External**. Internal people have an organization role of **Member** or **Administrator**. External people have no organization administration role.
4. Under **Direct workspace access**, choose workspaces and select an access role for each. An administrator can explicitly give an Internal person full access to all current and future workspaces.
5. Select **Create and invite**.

The invitation email lets the recipient set a password. The directory shows
**Invite pending** until they accept. Share [Accept an invitation](../for-clients/accept-invitation.md)
if they need help.

::: tip Choose the role before saving
Use External for clients and advisers. Account creation alone grants no document
access; a workspace role, group assignment or item share is needed. To share
just one file or folder, use its [Share](../sharing/share-items.md) action. To
invite someone to a whole workspace, open **Share workspace** at the client workspace root.
:::

## Find users and check invitations

Search or filter the **Users** directory. Opening **View Users** from a client filters the directory to that client. The status badge distinguishes **Invite pending** from **Accepted**.

For a pending invitation in the user directory, choose **Resend Invite**. For an
invitation made through sharing, expand **Pending** beside the person and use **Email invitation**.
On mobile, open **Manage access** and select the pending recipient.

Administrators can also copy account invitation or password-reset links from the
user directory. File and folder invitations are sent only to the recipient's email address.

Invitation and reset links provide account access. Send them directly to the person they were created for.

## Edit a user

1. As an administrator, choose **Edit** on the user's row.
2. Update the name, email, user type, organization role or direct workspace access as appropriate.
3. Save and check the directory.

An account may have no workspace assignments. Internal Member and External do
not grant document access on their own. Existing file/folder shares are preserved
when you edit workspace assignments. Removing a workspace assignment does not
remove access granted through a group.

Under **Settings → Access roles** (also linked from **Users**), administrators can
[customize the access-role catalog](../administration/access-roles.md). Changes to
existing roles require an impact review. Archived roles retain existing access
but cannot be assigned to new recipients. Group-management screens are not
available in this release; existing group access remains effective.

## Remove a user

Administrators can select **Delete** from the user's action dropdown to remove
their account. To remove access to just one client, open **Share workspace** at the
client workspace root, open the person's role and choose **Remove access**. Confirm
the removal, then select **Save**. On mobile, find **Share workspace** in the
**Upload** menu when uploads are available, then open **Manage access** and select
the person. You can also open workspace sharing from the client's **Share**
action in **Clients**.
This also removes their file and folder shares in that client and cancels their pending invitations.

Shares they created for other people keep working. If they have a sharing link,
they can still use it unless it is revoked. The removal dialog offers a separate
option to turn off every sharing link for the client.

If you are ending someone's access, also review any [secure links](../sharing/manage-links.md) they may hold. A link is a separate way to access a workspace.
