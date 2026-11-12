# Manage access roles

**Who:** Administrators.

Access roles define document permissions within a client workspace, folder or
file. They are separate from the **Member** and **Administrator** organization
roles. A role named Workspace manager does not grant portal administration.

Open **Settings → Access roles**, or **Users → Access roles**. Each role lists
its permissions, eligible people and scopes. The starting roles are editable;
check the summary rather than relying on the name.

## Create a role

1. Select **Create access role**.
2. Enter a **Name** and, optionally, a **Description**.
3. Choose **Available to**: Internal and External, Internal only, or External only.
4. Choose the **Applicable scopes**: Workspace, Folder and/or File.
5. Select the required **Permissions** and select **Create role**.

| Permission | Allows |
| --- | --- |
| View and download | Browse, preview and download shared documents. |
| Upload into folders | Add files and folders; never replace a single shared file. |
| Delete documents | Delete documents within the assigned scope. |
| Invite external people | Invite named recipients within the sharer's own permissions. |
| Create sharing links | Create links within the sharer's own permissions. |
| Manage people’s access | Manage assignments within the assigned scope and permission ceiling. |

Creating a role does not assign it. Choose it when inviting someone through
[sharing](../sharing/share-items.md) or editing their
[direct workspace access](../clients/manage-users.md). Only eligible roles within
the sharer's permissions appear. Upload permission has no effect on a single file.

## Edit an existing role

1. Select **Edit** beside the role.
2. Update its name, description or permissions.
3. Select **Review changes**.
4. Read the affected workspace memberships, item grants, invitations and additional
   assignments, then select **Confirm change**.

Permission changes take effect for existing recipients immediately. Renaming
changes only the label. Changing who an assigned role applies to, or its supported
scopes, requires a replacement role rather than narrowing the assigned role in place.

## Archive or delete a role

- **Archive:** review and confirm. Existing recipients keep their access, but the
  role is no longer offered for new assignments.
- **Delete:** choose an eligible **Replacement role** if the role has assignments,
  select **Review deletion**, then **Confirm change**. Check the replacement's
  permissions: recipients will use that role instead.

Archived access is not revoked access. To stop a person's access, remove the
assignment where it was granted and select **Save** in the sharing dialog.
Other access sources and secure links may still work.

## Upgraded portals

Named legacy roles preserve older permission combinations. Former Managers are
Internal Members with their migrated document access; some retain explicit legacy
organization management. Administrators can review that entitlement in **Users**.
Full access to all current and future workspaces is a separate setting, not a
consequence of a role's name.

Group-management screens are not available in this release. Existing group
assignments still contribute access and can appear in impact reviews.
