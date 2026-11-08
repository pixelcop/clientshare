# Scoped access implementation boundary

Migrations 050–054 introduce additive item sharing, scoped bearer links and
identity-bound invitations. The application registers scoped lifecycle routes
alongside the exact-resource checks in [the access contract](sharing-access-contract.md).
Item Share controls and Shared with me use server-provided capabilities; hiding
controls is never the authorization boundary.

File/folder operations, client/user management, current sessions, feed discovery
and resource-bound email all enforce current scope. Item grants never create
client membership, and scoped link registration cannot create whole-client access.

## Storage

- `item_grants` keeps one current row per tenant, recipient and item. Updating or
  reactivating access updates that row; non-secret before/after history belongs
  in `audit_events`. Composite foreign keys bind the recipient and target to the
  same tenant/client. Creator IDs are provenance, not revocation dependencies.
- `secure_links.file_id` is nullable. Existing links retain their tokens, modes,
  expiry, retry keys and whole-client scope. A target's file/folder type is derived
  from `files`, never stored as a separate editable scope type.
- `user_clients.capabilities_set=false` preserves legacy role-based membership
  rights. `true` uses independent `can_read/upload/delete/share` fields, including
  an explicit all-false membership. Legacy rows remain independent access
  sources; a later invitation must not replace that ownership. `invited_by` is
  meaningful only for invitation-owned membership, not independent membership.
- `users.manager_full_access` defaults to false. Migration 050 sets it true only
  for managers already present at upgrade, preserving access to future clients.
  Client lists, management, files, feeds and notifications enforce this policy.
- Migration 051 adds nullable `files.upload_session_id`. Private-collection
  recipients can address their explicit upload destination and newly created
  directory trees in the same authenticated session, not enumerate or reuse
  pre-existing child folders. Existing records remain unowned by a collection
  session. Directory continuation does not confer read permission.

Private collection returns empty file **and folder** lists and duplicate checks,
with only generic destination context. New file/folder names always receive an
opaque suffix, even when there is no collision. This avoids both overwrites and
a collision oracle. Named read/upload access retains ordinary name handling.
Recipient metadata and breadcrumb paths are derived only from the contiguous
authorized root, not the stored full path. Item access does not return the
client's name or private root metadata.

## Resolver

`services.AccessService` takes authenticated session IDs, tenant/client scope,
an exact item (or an explicit whole-client request), operation and optional time.
It reloads current users/links rather than trusting role strings or session
capability snapshots. Authentication and session lifetime remain auth middleware's
responsibility. Link identity takes precedence over named-user identity.

The implementation lives in dependency-light `internal/access`; the services
package re-exports it for application composition. Dispatchers use the same
resolver without depending on the higher-level email/service wiring.
`ClientScope` produces a tenant-safe whole-client SQL scope for listings and
management queries. It deliberately ignores item grants and reloads the current
user instead of accepting a role string as proof of authority. Client APIs apply
assigned/full manager policy; creating a client assigns its manager creator
transactionally, rather than granting the creator access to unrelated clients.

The staff user directory is scoped to manageable clients for managers, including
search, totals and returned client IDs. Managers can invite capability-bounded
client users within that scope, but cannot promote staff, change tenant-wide
identity settings, delete accounts, or generate account/password access links.
Those global identity operations are administrator-only; client membership
management is a separate scope from deleting a tenant account. Replacing client
assignments preserves unchanged capabilities/provenance and revokes only the
removed recipient's direct grants, not their onward shares or bearer links.
Legacy client-wide bearer registration, where supported, creates bounded
read/upload membership without named sharing authority. It never accepts an
item-targeted link as client membership.

Queue management also uses current assigned/full client scope. Identity setup
and password-reset jobs are administrator-only, even when their first client
matches a manager's assignment. Force processing is tenant-bound and visits only
jobs the caller can manage; it never invokes a global cross-tenant processor.
Its result means the jobs were processed/revalidated, not that every message was
delivered to an inbox.

Upload notifications bind each queued job to one named recipient or one bearer
link contact. Managers are eligible only within assigned/full client scope;
upload-only links receive no existing filenames. Direct item grants do not
automatically subscribe their recipients to client-wide uploads. Each affected
item is resolved at enqueue and again immediately before dispatch. The dispatcher
rebuilds names, counts and message bodies from currently readable, active items,
and cancels a job if none remain or the intended contact has changed.

Migration 052 adds this recipient/render context and cancels unsent legacy
upload batches that have no trustworthy recipient binding. It leaves sent mail,
identity invitations and ordinary secure-link delivery jobs intact. Operators
should review canceled pending upload notifications after upgrading and enqueue
fresh notifications where appropriate; do not send the old rendered bodies by
hand. Schema rollback does not reactivate canceled jobs. SMTP delivery cannot
be undone after a message has left the server; dispatch authorization is a
current check, not a promise of recalling previously sent mail.

Feed responses also resolve current exact-item access before pagination and
read/unread summaries. Grouped uploads include only authorized active files;
stored full paths, stale batch counts and inaccessible events are not returned.
Item-only context omits client/actor roster metadata. Read and read-all mutations
are restricted to currently visible events, and assigned-only managers are not
automatically subscribed to unrelated clients' uploads.

Migration 053 adds target-aware access invitations and optimistic versions for
current item grants. Grant creation is retry-safe but never silently changes or
reactivates an existing row; updates/revocation require its current version.
Invitations bind recipient email and tenant-local identity, are single-use, and
cannot create client membership for an item target. Independent membership is
never taken over or reduced by reinvitation. Account setup reuses existing
identities without changing their role, credentials or unrelated memberships.
New item-only identities start with no client assignments. Identity credentials
are mail-only and are not returned to the inviter when mail is unavailable.

Identity invitation acceptance claims the token, establishes credentials and
redeems any bound access intent transactionally. Revoked/expired/deleted targets
roll the transaction back. Old identity setup tokens cannot reset an account
that already has a password/passkey. Recipient removal revokes their direct
grants and pending access invitations within the client; onward shares survive.
Revoking known bearer links is a separate explicit option. Lifecycle routes
revalidate the actor's current scope and capability ceiling on every request.

Migration 054 binds queued access/setup invitations to current intent and
identity witnesses. Dispatch cancels expired, revoked, accepted or stale intent,
changed recipient addresses, deleted ancestry and obsolete setup credentials.
Generic queue inspection never returns these identity or acceptance credentials.
Resource-bound mail requires the database-backed queue; the in-memory fallback
refuses it rather than dispatching content without current authorization.

`ResolveItems` batches up to 500 exact items in one client, with parent/grant SQL
batches of 200. Parent traversal is limited to 128 nodes per chain, rejects
cycles, non-folder parents, foreign/missing/deleted ancestors and never uses
`path` prefixes. All decisions are uncached. Timestamp expiry is compared as an
instant in Go, including SQLite values stored with different offsets.

Recipient DTOs may expose effective capabilities, but must not serialize the
internal access sources or use raw model paths as navigation context. A folder
ZIP decision authorizes the selected folder, not unchecked archive entries.
Client invitations require a whole-client decision; even root-folder grants do
not confer membership. `CanGrant` checks a capability ceiling on an allowed
exact-scope decision; invitee ownership and staff-role assignment need separate
checks. Listing filtering/counts and scoped navigation remain caller obligations.

## Upgrade safety

Do not run an older binary once scoped links or assigned-only managers are in
use: it may treat an item link as client-wide or bypass manager assignments.
Prefer a forward corrective build. A schema rollback is not an access-safe
application rollback and discards grant storage; preserve a database backup.
See [the upgrade and recovery checklist](sharing-upgrade.md) before upgrading.
