# ClientShare user guide — proposed table of contents

Status: approved; guide implemented in `guide/` with Bun and VitePress 2.0.0-alpha.20.
Branch: `docs/user-guide`.

## Purpose and audience

A public, task-oriented guide linked as **User Guide** from the app navigation and, later, the SaaS website. The landing page helps readers choose **Manage a portal** or **Use a client portal**. Instructions use the app's actual labels and explain which role can perform each task.

The initial guide covers the shared portal experience for hosted and self-hosted users. Hosting-specific differences should be called out where relevant. Deployment documentation can remain linked from the guide; SaaS signup, billing, and subscription management can be added after those flows are reviewed.

## Proposed sidebar

Each numbered item below is one page. Sub-bullets describe that page's contents.

### Getting started

1. **Welcome to ClientShare**
   - What the portal does; choosing the staff or client guide.
   - How clients, users, folders, files, and secure links fit together.
2. **Your first client exchange**
   - Create a client, upload a document, and share access.
   - Collect a document back and check activity.
   - Link to the detailed instructions at each step.
3. **Sign in and find your way around**
   - Open your portal, sign in, and sign out.
   - Staff navigation versus the client file view.
   - Where to find the guide from inside the app.
4. **Roles and access explained**
   - Administrator, manager, client, and secure-link access.
   - A verified permissions table and explanations of missing actions.
   - Choosing between inviting a user and sending a secure link.

### Clients and users

5. **Create and manage clients**
   - Add a client, find a client, and edit client details.
   - Delete a client and understand the confirmation and consequences.
6. **Invite and manage users**
   - Add staff or client users, choose a role, and associate client access.
   - Invitations, pending invitations, and available resend actions.
   - Edit or remove users and understand the effect on access.

### Files and folders

7. **Organize client folders**
   - Open a client workspace, create folders, and navigate breadcrumbs.
   - Document the folder-management actions available in the current UI.
8. **Upload files**
   - Choose files or drag and drop; monitor completion and handle failures.
   - Notify the client by email when the option is available.
9. **Preview and download files**
   - Open previews and download documents.
   - Explain supported preview types and the download fallback.
10. **Delete files and folders**
    - Available deletion actions, required permissions, and confirmations.
    - Clearly describe permanence and folder contents based on verified behavior.

### Sharing and collecting documents

11. **Create and send a secure link**
    - Open link creation, select the intended access, set expiry, and share.
    - Send by email or copy the generated link.
12. **Choose link permissions and expiry**
    - Read, Write, and Read & Write explained with examples.
    - Expiry choices and the difference between expiry and revocation.
13. **Request documents from a client**
    - A complete upload-request workflow using a Write link.
    - What the recipient sees and how staff confirm the upload.
14. **Review and revoke secure links**
    - Locate a client's links and inspect their status.
    - Revoke access and provide a replacement link when needed.

### Guide for clients and recipients

15. **Accept an invitation**
    - Follow the invitation, set up access, and open the client portal.
    - What to do if an invitation is expired or unavailable.
16. **Open a secure link**
    - What happens after opening the link and any access prompts.
    - Explain the actions available for each link permission.
17. **Send and receive documents**
    - Find folders, upload requested files, preview, and download.
    - Confirm completion and contact the sender when access is missing.

### Activity and notifications

18. **Review activity**
    - Find the activity feed and interpret file and access events.
    - Explain visibility and any available filtering using the actual UI.
19. **Understand email notifications**
    - Invitations, secure-link emails, and upload notifications.
    - Administrator view of pending notifications and available controls.
    - First checks when an expected email does not arrive.

### Portal administration

20. **Customize portal branding**
    - Site title, logo, primary color, and branding preview.
    - Invitation welcome text.
21. **Configure portal settings**
    - Public base URL and default secure-link expiry.
    - Explain administrator-only settings and deployment-specific behavior.
    - Brief explanation of runtime log level if exposed in the deployed UI.

### Account security

22. **Reset your password**
    - Request a password reset and use the email link.
    - Missing or expired reset emails.
23. **Use passkeys**
    - Add a passkey, sign in with it, rename it, and remove it.
    - Device/browser prompts and what to do when a passkey is unavailable.

### Help and reference

24. **Troubleshooting**
    - Cannot sign in or open a link.
    - Cannot see a client, folder, file, or action.
    - Upload failed, preview unavailable, or email missing.
    - What information to provide to the portal administrator.
25. **Frequently asked questions**
    - Do recipients need an account?
    - What can the recipient see or change?
    - How do I stop sharing a document?
    - Which settings depend on my portal administrator?
26. **Glossary**
    - Portal, client, user, role, folder, secure link, expiry, and passkey.

## Writing and screenshot plan

- Each task page: short outcome, who can do it, prerequisites, numbered steps, expected result, and relevant troubleshooting links.
- Use concise role labels and UI names; verify permissions against both the UI and backend before publishing.
- Document implemented workflows. In particular, describe document requests through secure links rather than implying a separate request-tracking feature.
- Capture screenshots in Firefox after the TOC is approved, using the public demo reached through the homepage's **View sample portal** button.
- Initial screenshot targets: client directory, client workspace/folders, upload flow, file preview, secure-link dialog, link list, recipient file view, user invitation, activity, branding, and passkeys.
- Use consistent viewport dimensions, readable crops, descriptive alt text, and fictional sample content. Avoid displaying live access links or setup secrets.
- The initially empty demo at `https://demo.clientshare.app/clients` was populated with three fictional clients, nine folders, public sample documents, and invented text/CSV examples. Screenshot sources and capture details are recorded in `guide/demo-assets.md`.

## VitePress implementation notes

- Implemented with **2.0.0-alpha.20**, pinned in `guide/package.json` and locked with Bun.
- Upstream documents ESM configuration and installation through `vitepress@next`. This project runs development, build, and preview explicitly with **Bun**; the production build has been verified with Bun 1.4.2.
- Use the default theme with ClientShare branding, TypeScript configuration, grouped sidebar navigation, local full-text search, and previous/next links.
- Suggested site source: `guide/` with its own package scripts and `.vitepress/config.ts`. This provides a dedicated public content root alongside the existing project notes in `docs/`.
- Suggested URL: `docs.clientshare.app`, pending deployment approval. Both app navigation and the future SaaS navigation should link to the same public guide.
- Use stable task URLs, for example `/getting-started/first-client-exchange`, `/sharing/request-documents`, and `/for-clients/send-and-receive-documents`.
- Choose the deployment target before setting URL/base configuration and adding the app navigation link.

### Official documentation reviewed

- [Getting started](https://vitepress.dev/guide/getting-started)
- [Sidebar navigation](https://vitepress.dev/reference/default-theme-sidebar)
- [Search](https://vitepress.dev/reference/default-theme-search)

## Approval and tracking

- TOC approved. All 26 guide pages and the landing page are implemented; 13 screenshots from the populated demo are included.
- Verified the Bun production build, all 27 pages, 187 local links/anchors, all 13 images, desktop/mobile search, mobile sidebar navigation, dark mode, and horizontal overflow. The seven newly illustrated pages were also checked at a 390px mobile viewport. The app navigation change passes typechecking, targeted lint/format checks, and the existing app smoke test.
- The separate routing fix, `7c2d30a` on `fix/folder-role-navigation`, was deployed and verified. Folder, upload, PDF preview, recipient, and activity screenshots are now included. See `guide/demo-assets.md` for the sample content and capture notes.
- Public hosting/DNS for `docs.clientshare.app` remains to be configured. Build and publish instructions are in `guide/README.md`.
- Kanboard progress tracking is required by the repository guide. No usable board connection or project URL was available during this review; the open Kanboard browser tabs were product launch pages. Record this work in the appropriate project once access is provided.
