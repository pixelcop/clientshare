# Create and send a secure link

**Who:** Administrators.

A secure link lets a recipient exchange documents without first creating a user account. Decide which client workspace to share and what the recipient should be able to do before generating it.

## Create the link

1. Open **Clients**.
2. Open the client workspace, select **Manage client access**, then **Manage links**. The directory's **Links** action remains a shortcut.
3. Select **New link**.
4. Choose **Send documents** or **Collect uploads** as a starting preset, then review **Permissions**: **View & download**, **Upload only**, or **View, download & upload**. Selecting Collect always resets to Upload only.
5. Select or enter **Expires in (days)**, from 1 to 120. Review the absolute timestamp and timezone before confirming.
6. Confirm the client name, entire-workspace scope, audience, permissions and expiry in the access summary.
7. Select **Create link**. Creating it does not email it.

The result stays open with the created address, **Copy link**, **Open recipient view**, and a separate email action. You can also generate a link while creating a new client; its result opens on the client's links page.

::: important Review the entire workspace
The link applies to the client, including its folders. It is not restricted to the file or folder you last opened. Check that the workspace is appropriate for the recipient.
:::

## Copy and share

Select **Copy link** in the result, or **Copy** on a usable link's row. When the button reads **Copied**, paste it into your message to the intended recipient. Creation dates are plain text, not clipboard actions.

Include the purpose of the exchange, the documents you need, and the deadline. For example: “Please upload your project brief and supporting images using this link by Friday.”

If copying fails, select and copy the address in the result. Reopen the same result with **Details & email**. **Open recipient view** uses the existing confirmation before replacing a signed-in portal session; opening it does not silently use staff privileges.

## Email the existing link

Enter an address under **Email this existing link**, then select **Email link**. A retry uses that same link, not a newly created URL.

- **Email queued** means the configured queue accepted the message. It is not proof of delivery.
- **Accepted by the mail server** means a recorded send succeeded, not that the recipient read or received it.
- If email is unavailable or queueing fails, the result keeps the address. Copy it yourself or use **Retry email** after the problem is resolved.

Emailing a bearer link does not prove the recipient's identity or create named account access. Link email and upload-notification preferences are separate.

## Check the result

Confirm **Permissions** and **Expires** before sending. A new dialog starts from its explicit preset rather than silently restoring another client's saved permissions. Links are bearer credentials: anyone who obtains an active URL can use its permitted actions.

Next: [Choose permissions and expiry](./permissions-and-expiry.md), or [review and revoke links](./manage-links.md).
