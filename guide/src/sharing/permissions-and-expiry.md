# Choose link permissions and expiry

**Who:** Administrators creating links; recipients interpreting their access.

## Choose an access type

| Link option | Recipient label | Allows | Typical use |
| --- | --- | --- | --- |
| **View & download** | Read-only | View and download | Deliver completed documents |
| **Upload only** | Upload-only | Upload; downloads are disabled | Collect supporting documents |
| **View, download & upload** | Read & upload | View, download, and upload | Exchange drafts and responses |

These options do not give recipients staff administration or deletion controls. An upload-only link should not be used as a promise that filenames or the workspace structure are invisible.

Read-only recipients can browse folders, preview files, download files, and download folders as ZIP archives. They cannot upload or replace files, create folders, or delete anything.

The stored API modes remain `read`, `write`, and `readwrite`; legacy `view` is interpreted as `read`. Only **View, download & upload** links offer optional account registration. Read-only and upload-only recipients need an administrator's invitation to create an account; registering must not bypass the link's restrictions.

## Understand the scope

A link belongs to a **client workspace**, not a single document or subfolder. If two recipients should have different sets of documents, organize their material into appropriate client workspaces before sharing.

Someone who also has a user account may have separate access through that account. Link permissions describe the link session, not every access path the person could have.

## Set an expiry

In **New link**, choose a suggested number of days or type a value from **1 to 120**. The confirmation displays an exact expiry timestamp and timezone; the created result preserves that timestamp. The link becomes unusable after expiry, including in already-open sessions.

Portal settings provide the default expiry. New dialogs do not restore permissions from another client or sharing scope. **Send documents** and **Collect uploads** are presets, not additional permissions; always review the selected capability and timestamp.

## Expiry versus revocation

- **Expiry** ends access at the scheduled time.
- **Revoke** ends the link's access when you no longer want it used.

To change an existing link's permissions or provide more time, create a replacement with the intended settings and revoke the old one if it is still active. The link list has no edit action.

Neither expiry nor revocation removes copies already downloaded. See [Review and revoke links](./manage-links.md).
