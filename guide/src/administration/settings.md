# Configure portal settings

**Who:** Administrators.

Open **Settings**, make the intended changes, review the preview, and select **Save Changes**. Some fields appear only in self-hosted single-tenant deployments.

## Default secure-link expiry

Set **Default secure-link expiry** to a number of days from **1 to 120**. This supplies the starting value for link creation; the administrator creating a link can choose another value.

The link form can remember recent choices in that browser. Always check the expiry on the individual link. Changing the default does not extend or shorten links already created.

## Public base URL

**Single-tenant deployments only.** This is the public portal address used to construct invitation, password-reset, and secure-link URLs, for example `https://portal.example.com`.

Use the address recipients can actually open. The screen shows the **Current effective URL**. Confirm the value with the server operator before changing it, then verify newly generated links.

Hosted workspace addresses are managed by the hosted service, so this field is not shown there.

## Runtime log level

**Single-tenant deployments only.** This control changes the server's logging detail immediately. It resets when the server restarts.

Use it with the server operator when diagnosing a problem. It is not an email-notification preference or a setting for the Activity feed.

## Other settings on this page

- [Branding](./branding.md): title, logo, invitation text, and preview.
- [Email notifications](../activity/email-notifications.md): pending messages, refresh, and send-now controls.

Server storage, database, email credentials, and deployment configuration are managed outside this screen. See the [self-hosting documentation](https://github.com/pixelcop/clientshare#configuration) for those topics.

## User Guide destination

The **User Guide** menu link is configured by the server operator using `server.docs.url`. With no override, all portals open their built-in `/docs/` guide, regardless of tenancy mode. An external documentation site is used only when the operator explicitly configures one.

This setting is deployment-wide and is not edited on the portal Settings screen. Ask the server operator if your organization uses a different documentation address.

## Returning from the guide to your site

In a single-tenant portal's bundled guide, **Back to portal** uses the portal's public URL. A multi-tenant installation shows no return link until the operator chooses a destination, such as a central login or organization website.

The operator can customize or hide this link with `server.docs.site_link`. A separate docs container has its own `DOCS_SITE_LINK_*` settings. The ClientShare logo always leads to the guide's homepage.
