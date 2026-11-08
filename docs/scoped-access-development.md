# Scoped access implementation boundary

Migration 050 adds the storage foundation for item sharing. This does **not**
enable item sharing in the application. Until the protected-surface inventory in
[the access contract](sharing-access-contract.md) is fully enforced, item-link
creation is rejected and legacy bootstrap, session, inspection, registration and
email paths reject item-targeted links instead of treating them as client-wide.
There are no item-grant creation routes or item Share controls yet.

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
  Endpoint integration of this policy is a separate release gate.

## Resolver

`services.AccessService` takes authenticated session IDs, tenant/client scope,
an exact item (or an explicit whole-client request), operation and optional time.
It reloads current users/links rather than trusting role strings or session
capability snapshots. Authentication and session lifetime remain auth middleware's
responsibility. Link identity takes precedence over named-user identity.

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
