# ClientShare Core changelog

## Unreleased

### Added

- **Document requests:** Send clients a checklist by email or secure link. Clients can upload without an account; staff can accept each item, ask for a replacement, or add more items to the same request.
- **File conversations:** Discuss a document in its preview with formatted comments, @mentions, and email notifications. Staff can also leave private team notes.

### Changed

- Find recipients by name or email when sharing, inviting people, or sending a secure link.

### Fixed

- Queued emails on SQLite now send at the intended time even when their schedules use different time zones.
- SQLite file cleanup, support retries, and storage reservations now use the actual cutoff time even when existing timestamps have different offsets.

### Security

- Excluded document request pages from session replay and browser error reporting to protect private checklists and request links.
- SQLite now checks access, invitation, and session expiry by the actual instant, regardless of the stored time-zone offset.

## 1.0 — 2026-11-15

- **Branded client workspaces:** Organize, upload, preview, and download client files in a portal with your own logo and colors.
- **Secure file exchange:** Share workspaces, folders, or individual files with named recipients, or send expiring links for downloads or upload-only collection.
- **Access controls:** Define who can view, upload, or manage access at the workspace, folder, or file level. Invite users by email and let them sign in with a passkey or password.
- **Activity and notifications:** See recent document activity in the portal and get email updates.
- **Self-hosting:** Run a single service with SQLite and local storage by default, with PostgreSQL, MySQL, and S3-compatible storage available.
