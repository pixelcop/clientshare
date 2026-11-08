# Workspace sharing access contract

Implementation contract for additive client, folder and file sharing. This is
the target policy, not a claim that every milestone is available in a release.
Item sharing must not be enabled until all protected surfaces enforce it.

## Capabilities and scope

Capabilities are independent: `read` (metadata, list, preview, download),
`upload` (create files and folders), `delete`, and `share`. `readwrite` means
read plus upload, **not** delete, document editing or sharing. The legacy `view`
link mode remains an alias for `read`. File grants and file links cannot upload.
Sharing authority is a separate named-user capability, never a bearer-link
capability; a pure read-only grant cannot reshare unless sharing is separately
granted. Named recipients with sharing authority may reshare only their scope
and the capabilities they currently have.

| Identity/access source | Read | Upload | Delete | Share | Client management |
| --- | --- | --- | --- | --- | --- |
| Active tenant admin | All | All folders | All | All | All |
| Full-access manager | All | All folders | All | All | All clients; no admin/staff escalation |
| Assigned-only manager | Assigned clients and item grants | Assigned folders or upload grants | Assigned clients | Assigned clients or explicit sharing grants | Assigned clients only |
| Legacy client/customer membership | Client | Client folders | No | Client | Invite within own rights; manage own invitees only |
| Capability-bounded membership | Granted | Granted | Only if explicitly authorized | Granted | Own invitees within membership ceiling |
| Named file/folder grant | If granted | Folders if granted | Only if explicitly granted | Only if separately granted | None |
| Client/folder bearer link | read/view/readwrite | write/readwrite | Never | Never | None |
| File bearer link | read/view | Never | Never | Never | None |

Admin/manager role strings supplied by a caller are not proof of authority:
resolve the active tenant-local user and current manager-full-access state.
Existing managers migrate to explicit full access, including future clients;
new managers default to assigned-only. Legacy `customer` is equivalent to
`client` for membership resolution, not an all-client role.

Membership, ancestor-folder grants and direct grants combine additively. A
folder grant includes current and future descendants, never parents/siblings.
Root-folder grants are not memberships. A narrower grant cannot restrict a
member's inherited access. Link sessions use only their current link, even if
another browser session belongs to staff. No restricted/broken inheritance.

## Operation rules

| Operation | Required authority and response |
| --- | --- |
| Folder context | Read or upload on exact destination; minimal root context for upload-only |
| List, metadata, preview, download | Read on exact target; folder lists contain authorized children only |
| Breadcrumbs | Authorized folders only; stop at shared root; redact private path/parent IDs |
| Create folder/upload | Upload on actual destination, including every nested retry |
| Upload-only exists/list | No existing names, counts, IDs, folders or collision disclosures |
| Upload-only collisions | Allocate an opaque unique storage/logical name; never overwrite or expose existing names; new directory trees must not reuse private existing folders |
| ZIP | Read throughout selected subtree; omit deleted/unreadable content; no path-prefix authorization |
| Delete | Explicit delete on target; upload alone is insufficient |
| Grant/link create/update | Share on exact scope and requested rights no greater than actor's current rights |
| Client invitation | Membership-level sharing authority; no staff promotion or implicit membership from item access |
| Membership removal | Authorized staff or invitation owner; remove recipient's direct grants and pending invitations within that client atomically |
| Feed, collection, notifications | Revalidate resource access; no client-wide stream or counts for item-only recipients |
| Queue management | Authorized staff within assigned/full client scope, not any manager |

Inaccessible targets return the same not-found response as missing targets.
Expired/revoked grants, links, deleted targets/ancestors and deleted clients
fail the next protected request, including already-open sessions. Traverse
tenant/client-constrained `folder_id` relationships with cycle/depth protection;
do not trust `path`. No long-lived authorization cache.

## Lifecycle and management

- One current grant per tenant/item/recipient, with separate non-secret audit
  history. Validate tenant/client/target/user consistency transactionally.
- Invitations bind the intended email to a verified tenant-local account;
  acceptance is single-use/idempotent and must not create client membership for
  an item invitation or change an existing account's role.
- Record independent membership provenance. Reinviting an independent member
  cannot transfer management ownership to the inviter.
- Onward grants/links survive their creator's loss of access. That creator
  immediately loses management rights; do not recursively revoke other users.
- Membership removal does not invalidate known bearer links. Explain this and
  offer a separately authorized revoke-all-links operation for the client,
  including item targets. Pending invitations cannot restore removed access.
- No new move/copy APIs. Same-client reparenting preserves direct grants and
  recomputes ancestry; cross-client reparenting is rejected. New copies get new
  IDs and no explicit grants/links.
- Notification eligibility is item-aware. Direct grants do not automatically
  subscribe recipients to the whole client. Revalidate queued content before
  dispatch and partition batches by recipient visibility.
- Link creation and email delivery are separate, retry-safe operations. Return
  unavailable/failed/queued truthfully; queued is not delivered. Do not log
  bearer tokens or return recipient/provenance data outside management views.

## Protected-surface inventory

Registration is in `internal/clientshare/web.go`. Review these together before
enabling item sharing:

| Surface | Source | Enforcement integration |
| --- | --- | --- |
| `/api/folders`, `/:id`, `/exists`, `/breadcrumbs`, `/upload`, `/download-zip` | `handlers/files.go`, `middleware/auth.go` | Resolve exact target/default shared root, operation, redacted context and upload-only behavior |
| `/api/files/:id`, `/download`, DELETE | same; `services/file.go`, storage helpers | Exact-item resolver, safe subtree traversal, no inferred parent access |
| `/links/:token` GET/POST, `/api/link/:token` | `handlers/links.go`, `auth/sessions.go` | Current link and target validity; exact landing; preserve explicit session-switch confirmation |
| `/api/clients`, `/:id` GET/PUT/DELETE/POST | `handlers/clients.go`, `services/client.go` | Assigned/full manager scope and no item-grant membership promotion |
| `/api/users` and user assignment/invite/access-link endpoints | `handlers/users.go`, invitation/auth services | Scope, permission ceiling, provenance, no role escalation |
| `/api/clients/:id/links`, `/collection`; `/api/links/:id` | `handlers/links.go`, `services/collection.go` | Scope-aware creation/list/revocation/history and collection counts |
| `/api/feed`, `/read-all`, `/:id/read` | `handlers/feed.go`, `services/feed.go` | Filter visible resources and authorized counts before pagination |
| `/api/email/queue/*`; upload email enqueue/dispatch | `handlers/email_queue.go`, `handlers/files.go`, `services/email/` | Assigned-client management, item eligibility and dispatch revalidation |
| New `/access`, `/grants`, `/invitations`, item `/links`, link `/email`, `/shared-with-me` | sharing handlers/services | Same resolver, management-only recipient details, transactions and idempotency |

File delivery currently streams through `DownloadFileByPublicID` and the
temporary ZIP response; previews use authenticated downloads. Storage adapters
are server-side, not public file routes. No recipient presigned URL contract is
introduced. If one is added, document its bounded TTL and the fact that an
already-issued URL may outlive revocation. Static branding/guide delivery is
not a document authorization path.

## Release gates

1. Workspace upload/controller, loading/error/empty semantics and row actions.
2. Additive migrations, manager backfill, resolver matrix and tenant tests.
3. Every surface above, invitation races, notifications and discovery tests.
4. Only then enable item sharing UI/creation; validate nonmember file/folder
   recipients against parent/sibling/ZIP/roster/feed leaks.
5. Upgrade a representative database copy; verify old links/tokens/modes. Do
   not downgrade to a binary that treats item links as client-wide or ignores
   manager scope. Use a forward corrective build or neutralize incompatible
   access paths before rollback.

Optional combined listing is deferred: retain two groups in one compact
surface until there is a single deterministic paginated listing contract.
