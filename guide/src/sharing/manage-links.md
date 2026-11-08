# Review and revoke secure links

**Who:** Administrators.

## Find a client's links

Open the client workspace, then **Manage client access → Manage links**. The client's directory dropdown also retains **Links**.

The default **Usable links** view excludes expired and revoked links. Select **Expired & revoked history** to inspect retained records. History cannot reconstruct links that were permanently deleted by an older release.

The table shows:

| Column | Meaning |
| --- | --- |
| **Created** | When the link was generated. This is not a copy action. |
| **Permissions** | View & download, Upload only, or View, download & upload. |
| **Expires** | The expiry timestamp with timezone. |
| **Status** | Usable, Expired or Revoked; revoked history includes its revocation time. |
| **Actions** | Copy, open Details & email, or revoke a usable link. Unavailable links have no copy/email action. |

**Details & email → Link history** retains the creation time, last-access information when recorded, and any previously configured contact email. A contact email does not restrict bearer-link access. Link use or a queued email is not proof that a particular person read or approved a document.

## Revoke a link

1. Identify the link by its creation time, permissions and expiry.
2. Select **Revoke**.
3. Confirm the prompt explaining that this client-wide link will stop working while other links and account access remain unchanged.
4. Check **Expired & revoked history** for the retained revocation record.

Revocation removes that link's access on the next protected request, including already-open sessions. Unsent queued mail for it is canceled, and the queue rechecks link validity before sending. Mail already handed to SMTP cannot be recalled. If you revoke the wrong link, create a replacement; the old link cannot be reactivated through the UI.

## Replace an expired or unsuitable link

Create a [new link](./create-link.md) with the right permissions and expiry. Tell the recipient to use the new message. Revoke any old active link that should no longer be used.

::: tip Review other access too
A recipient with an assigned client-user account can still sign in separately. To end all intended access, also review [user assignments](../clients/manage-users.md). Copies already downloaded remain on the recipient's device.
:::
