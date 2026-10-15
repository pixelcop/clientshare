# Choose link permissions and expiry

**Who:** Administrators creating links; recipients interpreting their access.

## Choose an access type

| Link option | Recipient label | Allows | Typical use |
| --- | --- | --- | --- |
| **Read** | Read-only | View and download | Deliver completed documents |
| **Write** | Upload-only | Upload; downloads are disabled | Collect supporting documents |
| **Read & Write** | Read & upload | View, download, and upload | Exchange drafts and responses |

These options do not give recipients staff administration or deletion controls. An upload-only link should not be used as a promise that filenames or the workspace structure are invisible.

## Understand the scope

A link belongs to a **client workspace**, not a single document or subfolder. If two recipients should have different sets of documents, organize their material into appropriate client workspaces before sharing.

Someone who also has a user account may have separate access through that account. Link permissions describe the link session, not every access path the person could have.

## Set an expiry

In **New Secure Link**, choose a suggested number of days or type a value from **1 to 120**. The link becomes unusable after its expiry. Check the exact timestamp in the **Expires** column after creation.

Portal settings provide a default expiry, while the creation form may remember your previous choices in the browser. Always review the value for the current exchange.

## Expiry versus revocation

- **Expiry** ends access at the scheduled time.
- **Revoke** ends the link's access when you no longer want it used.

To change an existing link's permissions or provide more time, create a replacement with the intended settings and revoke the old one if it is still active. The link list has no edit action.

Neither expiry nor revocation removes copies already downloaded. See [Review and revoke links](./manage-links.md).
