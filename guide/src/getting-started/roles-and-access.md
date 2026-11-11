# Roles and access explained

**Who:** Anyone choosing how to share access or interpreting the actions they see.

## Identity and organization roles

- **Internal:** a member of your team. A **Member** has no document access just
  from having an account. An **Administrator** manages settings, people, roles
  and groups and retains access to all client workspaces.
- **External:** a client, adviser or other outside contact. External accounts
  have no organization administration role. Their access comes from assignments.
- **Link recipient:** opens shared files or uploads through a link, without an
  account. The sender chooses what the link allows.

## What people can do

Access roles belong to your organization. Administrators can create, rename and
customize them under **Roles and groups**. Roles are assigned to a person or group
at a workspace, folder or file. The starting templates are:

| Access role | Starting permissions |
| --- | --- |
| Viewer | View and download |
| Uploader | Upload only; existing files stay private |
| Contributor | View, download and upload |
| Client contact | View, upload and invite external people |
| File manager | View, upload and delete |
| Workspace manager | Document and access management; Internal only by default |

These are editable templates, not fixed permission names. Check the permission
summary before assigning a role. Invite external people, create sharing links,
manage access and delete documents are independent rights. You can grant only
permissions you currently hold. Upload applies to folder/workspace contents,
never to a single-file assignment.

Groups contain people, not other groups. Every member receives the group's
existing assignments, including future changes. Access adds across sources:
removing a direct role does not remove access inherited from a group or folder.

Sharing a client includes all its files and folders. Sharing a folder includes
everything inside it, including files added later. Sharing one file with someone
does not hide it from people who already have access to the client or folder.

| Task | Who can do it |
| --- | --- |
| View/download | People or links with view permission for those files |
| Upload/create folders | People or links with upload permission for that folder |
| Delete | Signed-in users with delete permission; links never allow deletion |
| Share files/folders | Signed-in users with permission to share them |
| Invite to the whole client | People with permission to share that client workspace |
| Remove direct access | People with manage-access permission, or the inviter for their own invitations |
| Change account details, user type or organization role | Administrators |
| Change portal settings | Administrators |

## Account or link?

Enter an email address in a file or folder's **Share** dialog when someone should
sign in to open it. After accepting, they can find it in **Shared with me**.
Use **Share workspace** at the client workspace root to invite someone to the
entire workspace. Inside a folder, return to the workspace root or use the
client's **Share** action in **Clients** for whole-workspace access. On mobile,
workspace and folder sharing are in the **Upload** menu when uploads are available.

A secure link works for anyone who has it. Choose the client, folder or file
carefully before sharing. File links allow viewing and downloading. Folder and
client links can also allow uploads. **Upload only** hides existing files and
folders. See [Share a file or folder](../sharing/share-items.md).

## Change or remove access

Role permission edits update existing assignments immediately. The review shows
affected sources before confirmation. Archiving a role prevents new assignments
but keeps existing access. Deleting an assigned role requires an eligible
replacement. Renaming a role changes its label, not its rights.

Upgraded installations retain previous permissions in named legacy roles where
necessary. Former Managers become Internal Members with their existing scoped
access and explicit legacy organization management. Administrators can turn off
that legacy entitlement in the person editor. All-current-and-future-workspace
authority remains an explicit setting, separate from individual scoped roles.

Removing someone from a file may leave access they have through the client or
another folder. Removing them from a client also removes their file and folder
shares in that client and cancels their pending invitations.
Group access remains until the person leaves the relevant group or the group's
scope assignment is removed. Review **Group and additional access** as well.

Shares they created for other people keep working. Links they already have also
keep working unless you revoke them. The removal dialog offers a separate option
to turn off every sharing link for the client. Files already downloaded cannot
be recalled.
