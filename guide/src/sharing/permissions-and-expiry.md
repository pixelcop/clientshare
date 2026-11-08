# Choose link permissions and expiry

**Who:** People creating links or interpreting their access.

## Choose an access type

| Link option | Recipient label | Allows | Typical use |
| --- | --- | --- | --- |
| **View & download** | Read-only | View and download | Deliver completed documents |
| **Upload only** | Upload-only | Upload; downloads are disabled | Collect supporting documents |
| **View, download & upload** | Read & upload | View, download, and upload | Exchange drafts and responses |

These options never grant sharing authority, staff administration or deletion. Upload-only is private collection: existing filenames, folders, counts and duplicate checks are not exposed. Incoming names receive opaque suffixes to avoid collisions without disclosure.

Read-only recipients can browse folders, preview files, download files, and download folders as ZIP archives. They cannot upload or replace files, create folders, or delete anything.

The stored API modes remain `read`, `write`, and `readwrite`; legacy `view` is interpreted as `read`. Only client-wide **View, download & upload** links offer optional account registration. Item links never create client membership; ask for a named invitation instead. Registration cannot bypass link restrictions or grant sharing authority.

## Understand the scope

A link targets a **client workspace**, **folder and descendants**, or **single file**. A folder link never grants access to its parents or siblings. File links are view/download only. Review the named target in the creation dialog; **Manage client access** always creates client-wide links.

Someone who also has a user account may have separate access through that account. Link permissions describe the link session, not every access path the person could have.

## Set an expiry

In an access dialog, select an expiry date/time. The staff **New link** shortcut accepts a suggested number of days or a value from **1 to 120**. Review the exact expiry timestamp and timezone. The link becomes unusable after expiry, including in already-open sessions.

Portal settings provide the default expiry. New dialogs do not restore permissions from another client or sharing scope. **Send documents** and **Collect uploads** are presets, not additional permissions; always review the selected capability and timestamp.

## Expiry versus revocation

- **Expiry** ends access at the scheduled time.
- **Revoke** ends the link's access when you no longer want it used.

To change an existing link's permissions or provide more time, create a replacement with the intended settings and revoke the old one if it is still active. The link list has no edit action.

Neither expiry nor revocation removes copies already downloaded. See [Review and revoke links](./manage-links.md).
