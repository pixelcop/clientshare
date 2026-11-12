# Share a file or folder

**Who:** People with permission to share the file or folder.

## Invite a person

1. Find the file or folder and select **Share** on its row or in **More**. On a
   narrow screen, use **More → Share**. File detail pages also have **Share**.
2. Check the name. Sharing a folder includes everything inside it.
3. In **Enter an email address**, enter an address and press **Enter** or select **Add**.
   Add more addresses in the input below the recipients.
4. Choose a role for each person. Use the clock beside their role to choose an
   expiry: 7 days, 30 days, or **Custom date…**. Custom access ends at the end of
   the selected day in your local timezone; confirm with **Set expiry**. The default
   is **No expiry**. A highlighted clock means an expiry is set. Hover for the
   exact date and time, or open the clock menu to see it at the top.
5. Select **Send invitation** and check the email status. If sending fails, expand
   **Pending** beside the invitation and use **Email invitation** to try again.
   On mobile, open **Manage access** and select the pending recipient for these options.

The recipient signs in with the invited email address, or follows the email to
set up an account. After accepting, they can find the file or folder in
**Shared with me**. This invitation does not give them the whole client workspace.

## Access shared from elsewhere

People may already have access through the client or a containing folder.
Removing them here will not remove that access. Open the client's **Share** dialog or
the containing folder's **Share** dialog to change it, if you have permission.

Select the role beside a person to change their role, or use the clock to change
an item assignment's expiry. Choose **Remove access** to end access added here
and confirm the removal. On mobile, open **Manage access**, select the person,
then choose their role, expiry or **Remove access**. Select **Save** in the sharing
dialog to apply role changes, expiry changes and removals. Closing without saving
lets you discard these edits. Shares they created for
other people keep working. Past invitations and removed access are kept in a
collapsed section below the current people, when there are past entries.

## Sharing roles

| Role | What the person can do |
| --- | --- |
| Viewer | View and download files. |
| Uploader | Upload to a folder or client workspace without seeing existing files or folders. |
| Contributor | View, download and upload files. |
| Client contact | View, upload and invite external people. |
| File manager | View, download, upload and delete files. A single-file assignment does not allow uploads. |
| Workspace manager | Document and access management; Internal only by default. |

These are tenant-editable starting templates. Their names do not grant organization
administration. Administrators manage them under **Settings → Access roles**. Only roles
applicable to the recipient type, target scope and your permission ceiling appear.
Uploader, Contributor and Client contact are not offered for individual files by
default. New invitations select a view-only or upload-only role when available;
otherwise choose permissions explicitly. To share with an existing Internal team
member, enter their account email address. Create new Internal identities in
**Users** first; sharing with a new address creates an External contact without
changing an existing account's identity type or organization role.

Existing combinations are preserved as named legacy roles during upgrade. Changing
expiry retains the role; selecting another role explicitly replaces its permissions.
An archived role keeps existing access but cannot be assigned to new recipients.
Access inherited from elsewhere is not reduced by a direct role change.

Group-management screens are not available in this release. Existing group and
additional assignments still contribute access. Removing a direct assignment
does not remove those other sources. See [Manage access roles](../administration/access-roles.md).

## Share a link

Select **Sharing links** at the bottom of the dialog, choose permissions and an
expiry, then select **Create link**. Use **Back to sharing** to return to people.
Anyone with the link can use it. File links allow viewing and downloading;
folder links can also allow uploads. **Upload only** hides existing files and folders.

Use **Copy link** to paste it into your own message. To send from the portal,
select **Email link** to reveal **Email link to**, enter an address and select
**Email link** beside it. A waiting email has not been sent yet.

Select the **Manage links** tab to copy an existing link or choose **Revoke** to
stop it working, even if someone already has it open. Revocation takes effect on
confirmation; it does not wait for **Save**. Under **Existing links**, **Show inactive links**
appears when there are expired or revoked links to include.

Inside a folder, **Share folder** shares that folder. To invite someone to the
whole client, return to the client workspace root and select **Share workspace**,
or use the client's **Share** action in **Clients**. On mobile, workspace and
folder sharing are in the **Upload** menu when uploads are available. Check the
dialog title to confirm what is included. Whole-workspace invitations do not
offer the per-person expiry clock used for file and folder invitations.
