# Invite and manage users

**Who:** Administrators and managers.

Create an individual user account for each person who needs ongoing access. A client workspace and a user account are separate records.

## Invite a user

1. Open **Users** and select **New User**, or choose **Add User** from a client's action dropdown.
2. Enter the person's **Name** and **Email**.
3. Administrators may choose **Admin**, **Manager**, or **Customer**. Managers can create Customer accounts only within their authorized client scope.
4. Choose **Assign Clients** for Customer or assigned-only Manager. Only an administrator can enable a manager's **Full access to all current and future clients**; new managers default to assigned-only. Creating a user from a client preselects that client.
5. Select **Create**.

An invitation email is sent so the recipient can set a password. The directory shows **Invite pending** until they accept. Share [Accept an invitation](../for-clients/accept-invitation.md) if they need help.

![New User dialog with the Customer role and Willow Creek Holdings selected under Assign Clients.](/screenshots/invite-client-user.png)

::: tip Choose the role before saving
Customer is the client-user role. Admin and Manager are staff roles, not a substitute for an external recipient's named share. Use [Share](../sharing/share-items.md) for one file/folder, or **Manage client access → Invite by email** for bounded whole-client access to a new or existing identity.
:::

## Find users and check invitations

Search or filter the **Users** directory. Opening **View Users** from a client filters the directory to that client. The status badge distinguishes **Invite pending** from **Accepted**.

For a pending legacy account invitation, use **Resend Invite**. Administrators alone can copy legacy account invitation/reset links. Scoped invitation/setup credentials are email-only and never returned to the inviter. Retry those with **Email invitation** in the access dialog.

Invitation and reset links provide account access. Send them directly to the person they were created for.

## Edit a user

1. As an administrator, choose **Edit** on the user's row.
2. Update the name, email, role, or client assignments as appropriate.
3. Save and check the directory.

The legacy Customer creation form requires an assignment. Item-only accounts instead come from named item invitations and need no membership. Managers manage workspace membership through **Manage client access**, not tenant-wide identity editing.

## Remove a user

Administrators can select **Delete** from the user's action dropdown to remove the tenant account. To remove only workspace access, use **Manage client access → Remove**. That also revokes the person's direct grants and pending invitations within that client, while their onward shares survive. The dialog offers explicit revocation of all client/file/folder bearer links; without that choice, known links may still work.

If you are ending someone's access, also review any [secure links](../sharing/manage-links.md) they may hold. A link is a separate way to access a workspace.
