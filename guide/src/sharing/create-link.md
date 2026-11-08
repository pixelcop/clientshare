# Create and send a secure link

**Who:** Named users with sharing authority on the intended scope.

A secure link lets a recipient exchange documents without first creating a user account. Decide whether to share a whole client, folder or file before generating it. For item scope, use [the item's Share dialog](./share-items.md).

## Create the link

1. Open **Clients**.
2. Open the client workspace and select **Manage client access**. Its **Links** section is explicitly whole-client scope.
3. Choose permissions and expiry in **Create a link**. The staff directory's **Links → New link** shortcut also remains available.
4. Choose **View & download**, **Upload only**, or **View, download & upload**, limited to your own rights. The directory shortcut also offers **Send documents** and **Collect uploads** presets; Collect resets to Upload only.
5. Review the absolute expiry timestamp and timezone. In the directory shortcut, **Expires in (days)** accepts 1 to 120.
6. Confirm the client name, entire-workspace scope, audience, permissions and expiry in the access summary.
7. Select **Create link**. Creating it does not email it.

The result stays open with the created address, **Copy link**, **Open recipient view**, and a separate email action. Authorized staff can also generate a link while creating a new client; its result opens on the client's links page.

::: important Review the entire workspace
A link created in **Manage client access** applies to the entire client. Opening a folder first does not narrow it. Use the item's **Share** control when only that file or folder should be shared.
:::

## Copy and share

Select **Copy link** in the result, or **Copy** on a usable link's row. When the button reads **Copied**, paste it into your message to the intended recipient. Creation dates are plain text, not clipboard actions.

Include the purpose of the exchange, the documents you need, and the deadline. For example: “Please upload your project brief and supporting images using this link by Friday.”

If copying fails, select and copy the address in the result. Reopen it with **Use link** in the access dialog, or **Details & email** in the directory shortcut. **Open recipient view** asks before replacing a signed-in portal session; it does not silently use staff privileges.

## Email the existing link

Enter an address under **Email this existing link**, then select **Email link**. A retry uses that same link, not a newly created URL.

- **Email queued** means the configured queue accepted the message. It is not proof of delivery.
- **Accepted by the mail server** means a recorded send succeeded, not that the recipient read or received it.
- If email is unavailable or queueing fails, the result keeps the address. Copy it yourself or use **Retry email** after the problem is resolved.

Emailing a bearer link does not prove the recipient's identity or create named account access. Link email and upload-notification preferences are separate.

## Check the result

Confirm **Permissions** and **Expires** before sending. A new dialog starts from its explicit preset rather than silently restoring another client's saved permissions. Links are bearer credentials: anyone who obtains an active URL can use its permitted actions.

Next: [Choose permissions and expiry](./permissions-and-expiry.md), or [review and revoke links](./manage-links.md).
