# Sharing upgrade and recovery

## Before upgrading

1. Stop writes and pause email dispatch for a consistent recovery point. Back up
   the database, configuration/secrets and file storage as a matching set. For an
   active SQLite database use SQLite's online backup API or `sqlite3 DATABASE
   '.backup BACKUP'`; copying only a live `.db` file can omit WAL transactions.
   Use the database vendor's consistent backup procedure for PostgreSQL.
2. Restore a copy into an isolated environment. Keep it off production domains,
   disable outbound email, and use independent storage/configuration. Protect
   backup credentials and personal data as carefully as the originals.
3. Run the new build's `make migrate` against that copy. Applied migrations are
   immutable; these changes are additive through migration 054. Do not rewrite
   or manually skip migration records.

## Compatibility changes to check

- Existing client memberships and legacy secure-link URLs, tokens, expiry and
  modes are retained. `view` remains a read alias. Null link item targets still
  mean the entire client; a file/folder target is never interpreted client-wide.
- Existing managers are backfilled to explicit full access, including future
  clients. Newly created managers are assigned-only unless an administrator
  deliberately enables full access. Review assignments and manager settings.
- Upload-only means private collection: no browsing existing files/folders,
  names, counts or duplicate probes. Incoming names gain opaque suffixes and
  cannot overwrite existing content. Read-plus-upload retains ordinary browsing.
- Migration 052 cancels unsent legacy upload batches lacking trustworthy
  recipient bindings. Review canceled jobs and enqueue fresh authorized
  notifications if needed; never send stale rendered bodies manually.
- Resource-bound mail needs the durable database-backed queue. Test delivery
  configuration separately; an accepted invitation or link creation response
  does not mean its email was delivered. Credentials remain email-only.
- Application HTTP logs redact bearer paths and omit URL queries/fragments;
  SQL logs omit bound parameters. Apply equivalent redaction to external proxy,
  analytics and monitoring logs, and protect older logs that may contain URLs.

## Validate the restored copy

Check a legacy client member and each link mode. Test a nonmember with one named
file share: it must not reveal parent/sibling content, parent ZIPs, client
rosters or private paths. Test a folder share, private-collection upload, expired
and revoked access in an already-open session, invitation acceptance and member
removal. Onward shares survive their creator's removal; known bearer links need
explicit revocation. Check assigned-only managers cannot list unassigned clients.

The repository's Go suite exercises SQLite/PostgreSQL migration fixtures,
invitation/removal races, scope enforcement and dispatch revalidation. Run
`go test ./...`, frontend tests/typecheck/build, and the isolated-preview browser
checks as part of release validation. Tests do not replace a restore rehearsal
using the installation's actual storage and email configuration.

## Upgrade and recover safely

Once the rehearsal succeeds, take a fresh consistent backup, migrate the real
database and start the matching new build. Verify health, legacy access, scoped
access and queue behavior before resuming normal use and email dispatch.

**Do not downgrade only the binary or schema after scoped access is used.** Older
code can treat an item link as client-wide or bypass assigned-only manager scope.
Prefer a forward corrective build. If a complete restore is unavoidable, stop
writes and restore the matching database, configuration and storage recovery
point together; account for lost post-backup uploads and access changes. Review
and revoke credentials that the restored snapshot may reactivate before exposing
the portal again. Previously downloaded files and SMTP-accepted messages cannot
be recalled by database restoration or access revocation.
