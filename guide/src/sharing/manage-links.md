# Review and revoke secure links

**Who:** Administrators.

## Find a client's links

Open **Clients**, then choose **Links** from the client's action dropdown.

![Secure Links list for Willow Creek Holdings showing an active Read & Write link with Copy and Revoke actions.](/screenshots/secure-links.png)

The table shows:

| Column | Meaning |
| --- | --- |
| **Link** | When the link was generated; selecting the text copies it. |
| **Access** | Read, Write, or Read & Write. |
| **Email** | Recipient address supplied when creating the link, if any. |
| **Expires** | The expiry date and time. |
| **Status** | Active or Expired. |
| **Last Accessed** | Last recorded link access, when available. |
| **Actions** | Copy or revoke the link. |

Last Accessed indicates link use; it is not proof that a person read or approved every document.

## Revoke a link

1. Identify the link by its creation time, access type, recipient, and expiry.
2. Select **Revoke**.
3. Confirm the browser prompt: “Revoke this link? This action cannot be undone.”
4. Check the refreshed list.

Revocation removes that link's access. If you revoke the wrong link, create a new one and send the replacement; the old link cannot be reactivated through the UI.

## Replace an expired or unsuitable link

Create a [new link](./create-link.md) with the right permissions and expiry. Tell the recipient to use the new message. Revoke any old active link that should no longer be used.

::: tip Review other access too
A recipient with an assigned client-user account can still sign in separately. To end all intended access, also review [user assignments](../clients/manage-users.md). Copies already downloaded remain on the recipient's device.
:::
